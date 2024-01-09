package model

type Priority int

const (
	HIGH_PRIORITY Priority = iota
	MEDIUM_PRIORITY
	LOW_PRIORITY
)

const PRIORITY_LEVEL_COUNT int = int(LOW_PRIORITY) + 1

type Bitrate int

const (
	LOW_BITRATE    Bitrate = 3
	MEDIUM_BITRATE Bitrate = 5
	HIGH_BITRATE   Bitrate = 10
)
