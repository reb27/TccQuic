package test_client

import (
	"log"
	"sync"
	"time"
)

type PlaybackSimulator struct {
	currentPlaybackSegment  int
	timePlaybackNextSegment time.Time
	mutex                   *sync.Mutex
	cond                    *sync.Cond

	segmentDuration time.Duration
	baseLatency     time.Duration
	firstSegment    int
	lastSegment     int
}

func NewPlaybackSimulator(
	segmentDuration time.Duration,
	baseLatency time.Duration,
	firstSegment int,
	lastSegment int,
) *PlaybackSimulator {
	mutex := new(sync.Mutex)
	cond := sync.NewCond(mutex)
	return &PlaybackSimulator{
		currentPlaybackSegment: firstSegment - 1,
		mutex:                  mutex,
		cond:                   cond,

		segmentDuration: segmentDuration,
		baseLatency:     baseLatency,
		firstSegment:    firstSegment,
		lastSegment:     lastSegment,
	}
}

func (p *PlaybackSimulator) Start() {
	p.mutex.Lock()
	timePlaybackNextSegment := time.Now().Add(p.baseLatency)
	p.timePlaybackNextSegment = timePlaybackNextSegment
	p.mutex.Unlock()

	go func() {
		time.Sleep(time.Until(timePlaybackNextSegment))

		for playbackSegment := p.firstSegment; playbackSegment <= p.lastSegment; playbackSegment++ {
			log.Printf("Playback %d", playbackSegment)

			p.mutex.Lock()
			timePlaybackNextSegment := time.Now().Add(p.segmentDuration)
			p.timePlaybackNextSegment = timePlaybackNextSegment
			p.currentPlaybackSegment = playbackSegment
			p.mutex.Unlock()
			p.cond.Broadcast()

			time.Sleep(time.Until(timePlaybackNextSegment))
		}
	}()
}

func (p *PlaybackSimulator) WaitForPlaybackStart(segment int) {
	p.mutex.Lock()
	for p.currentPlaybackSegment < segment {
		p.cond.Wait()
	}
	p.mutex.Unlock()
}

// Returns the time remanining until segment starts being played.
//
// Returns 0 if the segment was already played or if it is not currently in
// the buffer.
func (p *PlaybackSimulator) GetTimeToReceive(segment int) time.Duration {
	var result time.Duration = 0

	p.mutex.Lock()
	if segment == p.currentPlaybackSegment+1 {
		result = time.Until(p.timePlaybackNextSegment)
	}
	p.mutex.Unlock()

	if result < 0 {
		return 0
	} else {
		return result
	}
}
