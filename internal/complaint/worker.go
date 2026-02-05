// Package complaint handles complaint fetching and processing with concurrent workers.
package complaint

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"cmon/internal/telegram"

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
//   2. Process: Fetch details and send to Telegram
//   3. Result: Send result to results channel
//   4. Repeat: Continue until jobs channel closes
//   5. Stop: Exit when jobs channel is closed
type Worker struct {
	id      int                // Worker ID for logging
	jobs    <-chan Link        // Input: Complaints to process
	results chan<- ProcessResult // Output: Processing results
	ctx     context.Context    // Browser context for API calls
	tg      *telegram.Client   // Telegram client for notifications
	wg      *sync.WaitGroup    // WaitGroup for coordinated shutdown
}

// WorkerPool manages a pool of concurrent complaint processing workers.
//
// Benefits of worker pool:
//   - Controlled concurrency (prevents overwhelming the server)
//   - Resource reuse (workers stay alive between jobs)
//   - Backpressure handling (buffered job channel)
//   - Graceful shutdown (wait for all workers to finish)
//
// Performance:
//   - Sequential: 1 complaint/sec = 60 complaints/min
//   - 10 workers: 10 complaints/sec = 600 complaints/min
//   - 10x throughput improvement
//
// Configuration:
//   - Worker count: Configurable via config (default: 10)
//   - Job buffer: 100 (prevents blocking when submitting jobs)
//   - Result buffer: 100 (prevents blocking when collecting results)
type WorkerPool struct {
	workers    []*Worker
	jobs       chan Link
	results    chan ProcessResult
	wg         sync.WaitGroup
	workerCount int
}

// NewWorkerPool creates a new worker pool for concurrent complaint processing.
//
// Initialization:
//   1. Create buffered channels for jobs and results
//   2. Create worker instances
//   3. Start all workers in goroutines
//   4. Return pool for job submission
//
// Parameters:
//   - ctx: Browser context for API calls
//   - tg: Telegram client for notifications
//   - workerCount: Number of concurrent workers
//
// Returns:
//   - *WorkerPool: Ready-to-use worker pool
func NewWorkerPool(ctx context.Context, tg *telegram.Client, workerCount int) *WorkerPool {
	log.Printf("  → Creating worker pool with %d workers...\n", workerCount)

	pool := &WorkerPool{
		workers:     make([]*Worker, workerCount),
		jobs:        make(chan Link, 100),        // Buffer 100 jobs
		results:     make(chan ProcessResult, 100), // Buffer 100 results
		workerCount: workerCount,
	}

	// Create and start workers
	for i := 0; i < workerCount; i++ {
		worker := &Worker{
			id:      i + 1,
			jobs:    pool.jobs,
			results: pool.results,
			ctx:     ctx,
			tg:      tg,
			wg:      &pool.wg,
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
//
// This is non-blocking if the job buffer has space.
// If buffer is full, this will block until a worker picks up a job.
//
// Parameters:
//   - complaint: Complaint link to process
func (p *WorkerPool) Submit(complaint Link) {
	p.jobs <- complaint
}

// Close closes the job channel and waits for all workers to finish.
//
// Shutdown flow:
//   1. Close jobs channel (signals workers to stop after current job)
//   2. Wait for all workers to finish their current jobs
//   3. Close results channel (signals no more results coming)
//
// This ensures graceful shutdown with no lost work.
func (p *WorkerPool) Close() {
	close(p.jobs)    // No more jobs will be submitted
	p.wg.Wait()      // Wait for all workers to finish
	close(p.results) // No more results will be sent
}

// Results returns the results channel for collecting processed complaints.
//
// Usage:
//   for result := range pool.Results() {
//       // Handle result
//   }
//
// Returns:
//   - <-chan ProcessResult: Read-only results channel
func (p *WorkerPool) Results() <-chan ProcessResult {
	return p.results
}

// start begins the worker's processing loop.
//
// Worker loop:
//   1. Wait for job from jobs channel
//   2. Process job (fetch details, send to Telegram)
//   3. Send result to results channel
//   4. Repeat until jobs channel closes
//
// Error handling:
//   - Errors are logged and sent in result
//   - Worker continues processing next job
//   - No worker crashes from individual job failures
func (w *Worker) start() {
	defer w.wg.Done()

	log.Printf("  ✓ Worker #%d started\n", w.id)

	// Process jobs until channel closes
	for job := range w.jobs {
		log.Printf("  [Worker #%d] Processing complaint %s\n", w.id, job.ComplaintNumber)

		// Process the complaint
		result := w.processComplaint(job)

		// Send result
		w.results <- result

		if result.Error != nil {
			log.Printf("  [Worker #%d] ✗ Failed to process %s: %v\n", w.id, job.ComplaintNumber, result.Error)
		} else {
			log.Printf("  [Worker #%d] ✓ Processed %s successfully\n", w.id, job.ComplaintNumber)
		}
	}

	log.Printf("  ✓ Worker #%d stopped\n", w.id)
}

// processComplaint fetches complaint details and sends to Telegram.
//
// Processing flow:
//   1. Build API URL with complaint's API ID
//   2. Fetch complaint details via browser (uses session cookies)
//   3. Parse JSON response
//   4. Extract consumer name
//   5. Send formatted message to Telegram
//   6. Return result with message ID and consumer name
//
// Why use browser for API calls:
//   - Automatically includes session cookies
//   - No need to manage authentication tokens
//   - Same session as the browser automation
//
// Parameters:
//   - complaint: Complaint link with API ID
//
// Returns:
//   - ProcessResult: Result with message ID or error
func (w *Worker) processComplaint(complaint Link) ProcessResult {
	// Build API URL
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", complaint.APIID)

	var jsonResponse string

	// Fetch complaint details using browser context
	// This ensures session cookies are included automatically
	//
	// The async/await pattern is crucial:
	//   - fetch() returns a Promise
	//   - await ensures we wait for the response
	//   - WithAwaitPromise(true) tells ChromeDP to wait for Promise resolution
	err := chromedp.Run(w.ctx,
		chromedp.Evaluate(fmt.Sprintf(`
			(async function() {
				const response = await fetch('%s', {
					headers: { 'X-Requested-With': 'XMLHttpRequest' }
				});
				if (!response.ok) throw new Error('HTTP status ' + response.status);
				return await response.text();
			})()
		`, apiURL), &jsonResponse, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
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

	// Convert to pretty JSON for Telegram
	prettyJSON, _ := json.MarshalIndent(details, "  ", "  ")

	// Send to Telegram if client is configured
	var messageID string
	if w.tg != nil {
		msgID, err := w.tg.SendComplaintMessage(string(prettyJSON), complaint.ComplaintNumber)
		if err != nil {
			return ProcessResult{
				ComplaintID:  complaint.ComplaintNumber,
				ConsumerName: consumerName,
				Error:        fmt.Errorf("failed to send Telegram notification: %w", err),
			}
		}
		messageID = msgID
	}

	// Add small delay to avoid rate limiting
	// Telegram API limit: 30 messages/second
	// With 10 workers: 10 messages/second (safe margin)
	time.Sleep(100 * time.Millisecond)

	return ProcessResult{
		ComplaintID:  complaint.ComplaintNumber,
		MessageID:    messageID,
		ConsumerName: consumerName,
		Error:        nil,
	}
}
