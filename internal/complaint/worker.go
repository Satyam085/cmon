// Package complaint handles complaint fetching and processing with concurrent workers.
package complaint

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"cmon/internal/session"
)

// Worker represents a single worker in the complaint processing pool.
//
// Workers now use an HTTP session client instead of a ChromeDP browser context.
// They make direct authenticated API calls via session.Client.GetJSON().
type Worker struct {
	id      int
	jobs    <-chan Link
	results chan<- ProcessResult
	sc      *session.Client
	wg      *sync.WaitGroup
}

// WorkerPool manages a pool of concurrent complaint processing workers.
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
//   - sc: Authenticated session client (shared across all workers)
//   - workerCount: Number of concurrent workers
func NewWorkerPool(sc *session.Client, workerCount int) *WorkerPool {
	log.Printf("  → Creating worker pool with %d workers...\n", workerCount)

	pool := &WorkerPool{
		workers:     make([]*Worker, workerCount),
		jobs:        make(chan Link, 100),
		results:     make(chan ProcessResult, 100),
		workerCount: workerCount,
	}

	for i := 0; i < workerCount; i++ {
		worker := &Worker{
			id:      i + 1,
			jobs:    pool.jobs,
			results: pool.results,
			sc:      sc,
			wg:      &pool.wg,
		}
		pool.workers[i] = worker
		pool.wg.Add(1)
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
	close(p.jobs)
	p.wg.Wait()
	close(p.results)
}

// Results returns the results channel for collecting processed complaints.
func (p *WorkerPool) Results() <-chan ProcessResult {
	return p.results
}

// start begins the worker's processing loop.
func (w *Worker) start() {
	defer w.wg.Done()
	for job := range w.jobs {
		result := w.processComplaint(job)
		w.results <- result
		if result.Error != nil {
			log.Printf("  [Worker #%d] ✗ Failed to process %s: %v\n", w.id, job.ComplaintNumber, result.Error)
		}
	}
}

// processComplaint fetches complaint details via an authenticated HTTP GET.
//
// Processing flow:
//  1. Build API URL with complaint's API ID
//  2. GET complaint details via session client (session cookie sent automatically)
//  3. Parse JSON response
//  4. Extract consumer name
//  5. Return result with Details struct
func (w *Worker) processComplaint(complaint Link) ProcessResult {
	apiURL := fmt.Sprintf("https://complaint.dgvcl.com/api/complaint-record/%s", complaint.APIID)

	body, err := w.sc.GetJSON(apiURL)
	if err != nil {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("failed to fetch details: %w", err),
		}
	}

	if len(body) == 0 {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("API returned empty response"),
		}
	}

	var fullData map[string]interface{}
	if err := json.Unmarshal(body, &fullData); err != nil {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("failed to parse JSON: %w", err),
		}
	}

	complaintDetail, ok := fullData["complaintdetail"].(map[string]interface{})
	if !ok {
		return ProcessResult{
			ComplaintID: complaint.ComplaintNumber,
			Error:       fmt.Errorf("complaintdetail missing in API response"),
		}
	}

	details := Details{
		ComplainNo:      complaintDetail["complain_no"],
		ConsumerNo:      complaintDetail["consumer_no"],
		ComplainantName: complaintDetail["complainant_name"],
		MobileNo:        complaintDetail["mobile_no"],
		Description:     complaintDetail["description"],
		ComplainDate:    complaintDetail["complain_date"],
		ExactLocation:   complaintDetail["exact_location"],
		Area:            complaintDetail["area"],
	}

	consumerName := "Unknown"
	if details.ComplainantName != nil {
		consumerName = fmt.Sprintf("%v", details.ComplainantName)
	}

	// Small delay to avoid overwhelming the server
	time.Sleep(100 * time.Millisecond)

	return ProcessResult{
		ComplaintID:  complaint.ComplaintNumber,
		ConsumerName: consumerName,
		Details:      details,
		Error:        nil,
	}
}
