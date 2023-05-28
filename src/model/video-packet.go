package model

type VideoPacketRequest struct {
	Priority Priority
	Bitrate  Bitrate
	Segment  int
	Tile     int
}

type VideoPacketResponse struct {
	Priority Priority
	Bitrate  Bitrate
	Segment  int
	Tile     int
	Data     []byte
}
