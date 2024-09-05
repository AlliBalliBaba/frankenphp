package frankenphp

import (
	fswatch "github.com/dunglas/go-fswatch"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
)

// latency of the watcher in seconds
const watcherLatency = 0.15

var (
	watchSessions       []*fswatch.Session
	// we block reloading until workers have stopped
	blockReloading      atomic.Bool
	// when stopping the watcher we need to wait for reloading to finish
	reloadWaitGroup     sync.WaitGroup
	// the integrity ensures rouge events are ignored
	watchIntegrity      atomic.Int32
)

func initWatcher(watchOpts []watchOpt, workerOpts []workerOpt) error {
	if len(watchOpts) == 0 || len(workerOpts) == 0 {
		return nil
	}

	watchIntegrity.Store(watchIntegrity.Load() + 1)
	watchSessions := make([]*fswatch.Session, len(watchOpts))
	for i, watchOpt := range watchOpts {
		session, err := createSession(watchOpt, workerOpts)
		if err != nil {
			return err
		}
		watchSessions[i] = session
	}

	for _, session := range watchSessions {
		go session.Start()
	}

	reloadWaitGroup = sync.WaitGroup{}
	blockReloading.Store(false)
	return nil
}

func createSession(watchOpt watchOpt, workerOpts []workerOpt) (*fswatch.Session, error) {
	eventTypeFilters := []fswatch.EventType{
		fswatch.Created,
		fswatch.Updated,
		fswatch.Renamed,
		fswatch.Removed,
	}
	// Todo: allow more fine grained control over the options
	opts := []fswatch.Option{
		fswatch.WithRecursive(watchOpt.isRecursive),
		fswatch.WithFollowSymlinks(false),
		fswatch.WithEventTypeFilters(eventTypeFilters),
		fswatch.WithLatency(watcherLatency),
	}
	handleFileEvent := registerFileEvent(watchOpt, workerOpts, watchIntegrity.Load())
	return fswatch.NewSession([]string{watchOpt.dirName}, handleFileEvent, opts...)
}

func drainWatcher() {
	if len(watchSessions) == 0 {
		return
	}
	logger.Info("stopping watcher")
	stopWatcher()
	reloadWaitGroup.Wait()
}

func stopWatcher() {
	blockReloading.Store(true)
	watchIntegrity.Store(watchIntegrity.Load() + 1)
	for _, session := range watchSessions {
		if err := session.Stop(); err != nil {
			logger.Error("failed to stop watcher")
		}
		if err := session.Destroy(); err != nil {
			logger.Error("failed to destroy watcher")
		}
	}
}

func registerFileEvent(watchOpt watchOpt, workerOpts []workerOpt, integrity int32) func([]fswatch.Event) {
	return func(events []fswatch.Event) {
		for _, event := range events {
			if handleFileEvent(event, watchOpt, workerOpts, integrity) {
				break
			}
		}
	}
}

func handleFileEvent(event fswatch.Event, watchOpt watchOpt, workerOpts []workerOpt, integrity int32) bool {
	if !fileMatchesPattern(event.Path, watchOpt) || !blockReloading.CompareAndSwap(false, true) {
		return false
	}
	reloadWaitGroup.Wait()
	if integrity != watchIntegrity.Load() {
		return false
	}

	logger.Info("filesystem change detected, restarting workers...", zap.String("path", event.Path))
	go triggerWorkerReload(workerOpts)
	return true
}

func triggerWorkerReload(workerOpts []workerOpt) {
	reloadWaitGroup.Add(1)
	restartWorkers(workerOpts)
	reloadWaitGroup.Done()
	blockReloading.Store(false)
}
