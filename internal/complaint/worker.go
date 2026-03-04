// Package complaint handles complaint fetching and processing with concurrent workers.
package complaint

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Worker represents a single worker in the complaint processing pool.
//
// Architecture:
//   - Each worker runs in its own goroutine
//   - Workers pull jobs from a shared channel
//   - Results are sent to a results channel
//   - Errors are logged but don't stop the worker
//
// Lifecycle:
//   1. Start: Begin listening on jobs channel
//   2. Process: Fetch details via chromedp
//   3. Result: Send result to results channel
//   4. Repeat: Continue until jobs channel closes
//   5. Stop: Exit when jobs channel is closed
type Worker struct {
	id         int                   // Worker ID for logging
	jobs       <-chan Link           // Input: Complaints to process
	results    chan<- ProcessResult  // Output: Processing results
	ctx        context.Context       // Browser context for API calls
	wg         *sync.WaitGroup       // WaitGroup for coordinated shutdown
}

// WorkerPool manages a pool of concurrent complaint processing workers.
//
// Benefits of worker pool:
//   - Controlled concurrency (prevents overwhelming the server)
//   - Resource reuse (workers stay alive between jobs)
//   - Backpressure handling (buffered job channel)
//   - Graceful shutdown (wait for all workers to finish)
//
// Configuration:
//   - Worker count: Configurable via config (default: 10)
//   - Job buffer: 100 (prevents blocking when submitting jobs)
//   - Result buffer: 100 (prevents blocking when collecting results)
type WorkerPool struct {
	workers     []*Worker
	jobs        chan Link
	results     chan ProcessResult
	wg          sync.WaitGroup
	workerCount int
}

// NewWorkerPool creates a new worker pool for concurrent complaint processing.
//
// Parameters:
//   - ctx: Browser context for API calls
//   - workerCount: Number of concurrent workers
//
// Returns:
//   - *WorkerPool: Ready-to-use worker pool
func NewWorkerPool(ctx context.Context, workerCount int) *WorkerPool {
	log.Printf("  → Creating worker pool with %d workers...\n", workerCount)

	pool := &WorkerPool{
		workers:     make([]*Worker, workerCount),
		jobs:        make(chan Link, 100),          // Buffer 100 jobs
		results:     make(chan ProcessResult, 100), // Buffer 100 results
		workerCount: workerCount,
	}

	// Create and start workers
	for i := 0; i < workerCount; i++ {
		worker := &Worker{
			id:         i + 1,
			jobs:       pool.jobs,
			results:    pool.results,
			ctx:        ctx,
			wg:         &pool.wg,
		}

		pool.workers[i] = worker
		pool.wg.Add(1)

		// Start worker in goroutine
		go worker.start()
	}

	log.Printf("  ✓ Worker pool started with %d workers\n", workerCount)
	return pool
}

// Submit adds a complaint to the processing queue.
func (p *WorkerPool) Submit(complaint Link) {
	p.jobs <- complaint
}

// Close closes the job channel and waits for all workers to finish.
func (p *WorkerPool) Close() {
	close(p.jobs)    // No more jobs will be submitted
	p.wg.Wait()      // Wait for all workers to finish
	close(p.results) // No more results will be sent
}

// Results returns the results channel for collecting processed complaints.
func (p *WorkerPool) Results() <-chan ProcessResult {
	return p.results
}

// start begins the worker's processing loop.
func (w *Worker) start() {
	defer w.wg.Done()

	// Process jobs until channel closes
	for job := range w.jobs {
		// Process the complaint
		result := w.processComplaint(job)

		// Send result
		w.results <- result

		if result.Error != nil {
			log.Printf("  [Worker #%d] ✗ Failed to process %s: %v\n", w.id, job.ComplaintNumber, result.Error)
		}
	}
}

// processComplaint fetches complaint details.
//
// Processing flow:
//   1. Build API URL with complaint's API ID
//   2. Fetch complaint details via browser (uses session cookies)
//   3. Parse JSON response
//   4. Extract consumer name
//   5. Return result with Details struct to be processed later
//
// Parameters:
//   - complaint: Complaint link with API ID
//
// Returns:
//   - ProcessResult: Result containing the details or error
func (w *Worker) processComplaint(complaint Link) ProcessResult {
	// Build API URL
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", complaint.APIID)

	var jsonResponse string

	// Create context with timeout for worker to prevent silent cancellations
	workerCtx, cancel := context.WithTimeout(w.ctx, 30*time.Second)
	defer cancel()

	apiURLJSON, _ := json.Marshal(apiURL)

	// Fetch complaint details using browser context
	err := chromedp.Run(workerCtx,
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				const response = await fetch(%s, {
					headers: { 'X-Requested-With': 'XMLHttpRequest' }
				});
				if (!response.ok) throw new Error('HTTP status ' + response.status);
				return await response.text();
			})()
		`, apiURLJSON), &jsonResponse, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)

	if err != nil {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("failed to fetch details: %w", err),
		}
	}

	if jsonResponse == "" {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("API returned empty response"),
		}
	}

	// Parse JSON response
	var fullData map[string]interface{}
	if err := json.Unmarshal([]byte(jsonResponse), &fullData); err != nil {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("failed to parse JSON: %w", err),
		}
	}

	// Extract complaint details from nested structure
	var details Details
	if complaintDetail, ok := fullData["complaintdetail"].(map[string]interface{}); ok {
		details = Details{
			ComplainNo:      complaintDetail["complain_no"],
			ConsumerNo:      complaintDetail["consumer_no"],
			ComplainantName: complaintDetail["complainant_name"],
			MobileNo:        complaintDetail["mobile_no"],
			Description:     complaintDetail["description"],
			ComplainDate:    complaintDetail["complain_date"],
			ExactLocation:   complaintDetail["exact_location"],
			Area:            complaintDetail["area"],
		}
	} else {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("complaintdetail missing in API response"),
		}
	}

	// Extract consumer name for storage
	consumerName := "Unknown"
	if details.ComplainantName != nil {
		consumerName = fmt.Sprintf("%v", details.ComplainantName)
	}

	// Add small delay to avoid overwhelming the target server locally
	time.Sleep(100 * time.Millisecond)

	return ProcessResult{
		ComplaintID:  complaint.ComplaintNumber,
		ConsumerName: consumerName,
		Details:      details,
		Error:        nil,
	}
}
