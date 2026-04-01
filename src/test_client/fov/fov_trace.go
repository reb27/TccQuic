package fov

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type FOVTrace struct {
	framesPerSegment int
	tilesBySegment   map[int]map[int]struct{}
	maxSegment       int
}

func LoadFOVTrace(path string, fps int, segmentDuration time.Duration) (*FOVTrace, error) {
	if fps <= 0 {
		return nil, fmt.Errorf("invalid fps=%d", fps)
	}
	if segmentDuration <= 0 {
		return nil, fmt.Errorf("invalid segmentDuration=%s", segmentDuration)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	framesPerSegment := int(segmentDuration.Seconds()*float64(fps) + 0.5)
	if framesPerSegment <= 0 {
		return nil, fmt.Errorf("segmentDuration=%s and fps=%d yield framesPerSegment=%d", segmentDuration, fps, framesPerSegment)
	}

	trace := &FOVTrace{
		framesPerSegment: framesPerSegment,
		tilesBySegment:   make(map[int]map[int]struct{}),
	}

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNo++

		if lineNo == 1 && strings.Contains(line, "frames") {
			continue
		}
		if len(line) == 0 {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}

		frameStr := strings.TrimSpace(fields[0])
		frameIdx, err := strconv.Atoi(frameStr)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid frame number %q: %w", lineNo, frameStr, err)
		}
		if frameIdx <= 0 {
			continue
		}
		segmentIdx := (frameIdx-1)/framesPerSegment + 1
		if segmentIdx <= 0 {
			continue
		}

		segmentSet := trace.tilesBySegment[segmentIdx]
		if segmentSet == nil {
			segmentSet = make(map[int]struct{})
			trace.tilesBySegment[segmentIdx] = segmentSet
		}

		for _, token := range fields[1:] {
			val := strings.TrimSpace(token)
			if val == "" {
				continue
			}
			tileID, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid tile number %q: %w", lineNo, val, err)
			}
			if tileID <= 0 {
				continue
			}
			segmentSet[tileID] = struct{}{}
		}

		if segmentIdx > trace.maxSegment {
			trace.maxSegment = segmentIdx
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return trace, nil
}

func (t *FOVTrace) TilesForSegment(segment int) []int {
	if t == nil || segment <= 0 {
		return nil
	}
	set := t.tilesBySegment[segment]
	if len(set) == 0 {
		return nil
	}
	result := make([]int, 0, len(set))
	for tile := range set {
		result = append(result, tile)
	}
	return result
}

func (t *FOVTrace) Contains(segment int, tile int) bool {
	if t == nil || segment <= 0 {
		return false
	}
	set := t.tilesBySegment[segment]
	if set == nil {
		return false
	}
	_, ok := set[tile]
	return ok
}

func (t *FOVTrace) MaxSegment() int {
	if t == nil {
		return 0
	}
	return t.maxSegment
}
