package downloader

import "time"

const progressReportInterval = time.Second

type progressCounter struct {
	completed int64
	report    func(completed int64)
	lastSent  time.Time
}

func (counter *progressCounter) Write(content []byte) (int, error) {
	counter.completed += int64(len(content))
	if counter.report != nil && time.Since(counter.lastSent) >= progressReportInterval {
		counter.lastSent = time.Now()
		counter.report(counter.completed)
	}
	return len(content), nil
}

type fileProgressReporter struct {
	manager *Manager
	jobID   string
	path    string
}

func (reporter *fileProgressReporter) report(completed int64) {
	if err := reporter.manager.store.UpdateFileProgress(reporter.jobID, reporter.path, completed); err != nil {
		reporter.manager.logRuntime("download progress could not be saved job=%s path=%q error=%q", reporter.jobID, reporter.path, err)
		return
	}
	reporter.manager.publishCurrent(reporter.jobID)
}
