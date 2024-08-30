package processor

import (
	"fmt"
	"sync"

	"github.com/vitali-fedulov/images4"
)

const (
	ScoreSimilar        float32 = 0.001 // score to assign for similar images
	ScoreDifferent      float32 = 1.0   // score to assign for different images
	SimilarityThreshold float32 = 0.5   // maximum score for similar images
)

type fcmpRequest struct {
	frameID1 int
	frameID2 int
}

type fcmpResponse struct {
	frameID1 int
	frameID2 int
	score    float32
	err      error
}

func (req fcmpRequest) String() string {
	return fmt.Sprintf("<fcmpRequest: %d <> %d >", req.frameID1, req.frameID2)
}

func (rsp fcmpResponse) String() string {
	if rsp.err == nil {
		return fmt.Sprintf("<cmp result: %d <> %d = %f >", rsp.frameID1, rsp.frameID2, rsp.score)
	}

	return fmt.Sprintf("<cmp ERROR: %d <> %d: %s >", rsp.frameID1, rsp.frameID2, rsp.err)
}

func (proc *Processor) compareFrames() error {
	var wg sync.WaitGroup
	requestQueue := make(chan fcmpRequest)
	responseQueue := make(chan fcmpResponse)

	for ii := 1; ii <= proc.numWorkers; ii++ {
		wg.Add(1)
		go proc.fcmpWorker(ii, requestQueue, responseQueue, &wg)
	}

	go proc.generateComparisonJobs(requestQueue)
	go func() { // wait for workers to finish, then close the response channel
		wg.Wait()
		close(responseQueue)
		proc.logger.Debug("All workers are done")
	}()
	proc.processComparisonResults(responseQueue)
	proc.logger.Debugf("Done comparing frames")

	for frameID, bucket := range proc.frameBuckets {
		proc.groups[bucket] = append(proc.groups[bucket], frameID)
	}

	return nil
}

func (proc *Processor) generateComparisonJobs(requestQueue chan fcmpRequest) {
	numFrames := len(proc.frames)
	proc.stats.NumTotalComparisons = numFrames * (numFrames - 1) / 2

	for ii := range numFrames {
		frameID1 := proc.frames[ii]

		for jj := range ii {
			frameID2 := proc.frames[jj]
			score, found := proc.state.GetComparisonScore(frameID1, frameID2)

			if found {
				proc.bucketResults(frameID1, frameID2, score)
				proc.stats.NumCacheHits++
				proc.stats.IncNumComparisonsMade()
				continue
			}

			req := fcmpRequest{frameID1: frameID1, frameID2: frameID2}
			requestQueue <- req
		}
	}

	close(requestQueue)
	proc.logger.Debugf("All comparison jobs sent")
}

func (proc *Processor) processComparisonResults(responseQueue chan fcmpResponse) {
	numResponses := 0

	for response := range responseQueue {
		proc.logger.Debugf("Received comp result: %s", response)
		numResponses++

		if response.err == nil {
			proc.state.SetComparisonScore(response.frameID1, response.frameID2, response.score)
			proc.bucketResults(response.frameID1, response.frameID2, response.score)
		}

		proc.stats.IncNumComparisonsMade()
	}

	proc.logger.Debugf("Done processing %d responses", numResponses)
}

func (proc *Processor) fcmpWorker(workerID int, requestQueue chan fcmpRequest, responseQueue chan fcmpResponse, wg *sync.WaitGroup) {
	defer wg.Done()

	for req := range requestQueue {
		file1 := proc.state.GetFrameFileName(req.frameID1)
		file2 := proc.state.GetFrameFileName(req.frameID2)
		score, err := proc.compareImageFiles(file1, file2)

		if err != nil {
			proc.logger.Errorf("Worker %d: comparison error: %d <> %d: %s", workerID, req.frameID1, req.frameID2, err)
		}

		responseQueue <- fcmpResponse{frameID1: req.frameID1, frameID2: req.frameID2, score: score, err: err}
	}
}

func (proc *Processor) compareImageFiles(imageFile1 string, imageFile2 string) (float32, error) {
	img1, err := images4.Open(imageFile1)

	if err != nil {
		proc.logger.Errorf("Failed to open image file %s: %v", imageFile1, err)
		return 0, err
	}

	img2, err := images4.Open(imageFile2)

	if err != nil {
		proc.logger.Errorf("Failed to open image file %s: %v", imageFile2, err)
		return 0, err
	}

	// Icons are compact hash-like image representations.

	icon1 := images4.Icon(img1)
	icon2 := images4.Icon(img2)

	if images4.CustomSimilar(icon1, icon2,
		images4.CustomCoefficients{Y: proc.ChrTolerance, Cb: proc.ChrTolerance, Cr: proc.ChrTolerance, Prop: proc.PropTolerance}) {
		proc.logger.Debugf("SIMILAR: %s and %s", imageFile1, imageFile2)
		return ScoreSimilar, nil
	}

	return ScoreDifferent, nil
}

func (proc *Processor) bucketResults(frameID1, frameID2 int, score float32) {
	if proc.isFalsePositive(score) {
		proc.stats.NumFalsePositives++
	} else if score <= SimilarityThreshold {

		// determine the bucket to assign frames to

		var resultingBucket int

		if bucket, found := proc.frameBuckets[frameID1]; found {
			resultingBucket = bucket
		} else if bucket, found := proc.frameBuckets[frameID2]; found {
			resultingBucket = bucket
		} else {
			resultingBucket = proc.newBucket()
		}

		proc.bucketMutex.Lock()
		proc.frameBuckets[frameID1] = resultingBucket
		proc.frameBuckets[frameID2] = resultingBucket
		proc.stats.NumMatches++
		proc.bucketMutex.Unlock()
	}
}

func (proc *Processor) isFalsePositive(score float32) bool {
	return score < 0 && !proc.IgnoreFalsePositives
}
