package processor

import (
	"container/list"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type fgRequest struct {
	id             int
	videoFile      string
	frameID        int
	frameImageFile string
}

// The response is only sent back when generation failed

type fgResponse struct {
	frameID int
	err     error
}

func (req fgRequest) String() string {
	return fmt.Sprintf("<fgRequest: '%s' -> '%s' >", req.videoFile, req.frameImageFile)
}

func (rsp fgResponse) String() string {
	return fmt.Sprintf("<FG error frame #%d [%s]>", rsp.frameID, rsp.err)
}

func (proc *Processor) generateFrames(directories []string) error {
	var wg sync.WaitGroup
	requestQueue := make(chan fgRequest)
	responseQueue := make(chan fgResponse)

	for ii := 1; ii <= proc.numWorkers; ii++ {
		wg.Add(1)
		go proc.fgWorker(ii, requestQueue, responseQueue, &wg)
	}

	frames := make(map[int]bool)
	go proc.fgSendJobs(directories, requestQueue, &frames)

	failedFrames := list.New()
	go proc.fgProcessResults(responseQueue, failedFrames)
	wg.Wait()

	// delete failed files from the table

	for e := failedFrames.Front(); e != nil; e = e.Next() {
		frameID := e.Value.(int)
		delete(frames, frameID)
	}

	// Save the list of all frames we will be processing

	proc.frames = make([]int, len(frames))
	idx := 0

	for frameID := range frames {
		proc.frames[idx] = frameID
		idx++
	}

	proc.logger.Debugf("Done generating frames")
	return nil
}

func (proc *Processor) fgSendJobs(directories []string, requestQueue chan fgRequest, frames *map[int]bool) {
	for _, dir := range directories {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || !proc.isEligibleFile(path) {
				return nil
			}

			if proc.UseAbsolutePaths {
				orgpath := path
				path, err = normalizePath(path)

				if err != nil {
					proc.logger.Fatalf("Failed to normalize path for '%s': %v", orgpath, err)
				}
			}

			frameID, found := proc.state.RegisterFile(path)
			(*frames)[frameID] = true
			frameFile := proc.state.GetFrameFileName(frameID)
			_, err = os.Stat(frameFile)

			if !found || err != nil {
				proc.logger.Debugf("file '%s' has no frame", path)
				proc.stats.NumFramesToGenerate++
				req := fgRequest{frameID: frameID, videoFile: path, frameImageFile: frameFile}
				requestQueue <- req
			} else {
				proc.stats.IncNumFilesGenerated()
			}

			return nil
		})
	}

	close(requestQueue)
	proc.logger.Debugf("all frame generation jobs sent")
}

func (proc *Processor) fgProcessResults(responseQueue chan fgResponse, failedFrames *list.List) {
	for response := range responseQueue {
		proc.logger.Debugf("Received result: %s", response)

		if response.err == nil {
			proc.logger.Error("Received no-error response from frame generating worker!")
		}

		failedFrames.PushBack(response.frameID)
	}
}

func (proc *Processor) fgWorker(workerID int, requestQueue chan fgRequest, responseQueue chan fgResponse, wg *sync.WaitGroup) {
	defer wg.Done()

	for req := range requestQueue {
		err := proc.generateFrame(req.videoFile, req.frameImageFile)

		if err != nil {
			proc.logger.Errorf("Worker %d: failed to generate frame for '%s': %s", workerID, req.videoFile, err)
			responseQueue <- fgResponse{frameID: req.frameID, err: err}
		}

		proc.stats.IncNumFilesGenerated()
	}
}

// We try to generate a frame at 10s but if it failes (e.g. video is short)
// we fall back to a frame at 3rd second

func (proc *Processor) generateFrame(path string, frameFile string) error {
	offsets := []string{"00:10", "00:03", "00:01"}
	var err error

	for _, offset := range offsets {
		err = proc.generateFrameAtOffset(path, frameFile, offset)

		if err == nil {
			return nil
		}

		proc.logger.Warningf("Failed to generate frame file for '%s' at offset %s", path, offset)
	}

	return err
}

// The actual frame generation logic

func (proc *Processor) generateFrameAtOffset(path string, frameFile string, offset string) error {
	proc.logger.Debugf("Generating frame at offset %s: %s -> %s", offset, path, frameFile)

	program := "ffmpeg"
	args := []string{
		"-loglevel",
		"quiet",
		"-y",
		"-ss",
		offset,
		"-i",
		path,
		"-frames:v",
		"1",
		"-q:v",
		"2",
		"-s",
		"400x400",
		frameFile}
	cmd := exec.Command(program, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("ffmpeg failed (%d)", exitError.ExitCode())
		}

		return errors.New("failed to run ffmpeg")
	}

	if _, err = os.Stat(frameFile); err != nil {
		return errors.New("failed to generate frame file")
	}

	return nil
}

func (proc *Processor) isEligibleFile(path string) bool {
	videoExtensions := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".wmv": true,
		".flv": true, ".webm": true, ".ogg": true, ".ogv": true, // Add more as needed
	}

	ext := strings.ToLower(filepath.Ext(path))

	if !videoExtensions[ext] { // not a video file
		return false
	}

	if proc.exclusionRx != nil {
		return !proc.exclusionRx.MatchString(path)
	}

	return true
}

func normalizePath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return relativePath, nil
	}

	cwd, err := os.Getwd()

	if err != nil {
		return "", fmt.Errorf("error getting working directory: %v", err)
	}

	fullPath := filepath.Join(cwd, relativePath)

	normalizedPath, err := filepath.Abs(fullPath)

	if err != nil {
		return "", fmt.Errorf("error normalizing path: %v", err)
	}

	return normalizedPath, nil
}
