package test_client

import (
	"log"
	"sync"
	"time"
)

type PlaybackSimulator struct {
	currentPlaybackSegment int
	mutex                  *sync.Mutex
	cond                   *sync.Cond

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
	go func() {
		time.Sleep(p.baseLatency)

		for playbackSegment := p.firstSegment; playbackSegment <= p.lastSegment; playbackSegment++ {
			log.Printf("Playback %d", playbackSegment)

			p.mutex.Lock()
			p.currentPlaybackSegment = playbackSegment
			p.mutex.Unlock()
			p.cond.Broadcast()

			time.Sleep(p.segmentDuration)
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

func (p *PlaybackSimulator) CurrentBufferSegment() int {
	p.mutex.Lock()
	currentBufferSegment := p.currentPlaybackSegment + 1
	p.mutex.Unlock()
	return currentBufferSegment
}
