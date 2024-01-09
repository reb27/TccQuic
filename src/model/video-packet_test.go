package model_test

import (
	"bufio"
	"bytes"
	"main/src/model"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	(&model.VideoPacketRequest{
		Priority: 1,
		Bitrate:  2,
		Segment:  3,
		Tile:     4,
	}).Write(buf)
	expected := []byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4

`)

	assert.Equal(t, expected, buf.Bytes())
}

func TestReadRequest(t *testing.T) {
	buf := bytes.NewBuffer([]byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4

`))
	req, err := model.ReadVideoPacketRequest(bufio.NewReader(buf))

	assert.NotNil(t, req)
	assert.Nil(t, err)

	assert.Equal(t, 1, int(req.Priority))
	assert.Equal(t, 2, int(req.Bitrate))
	assert.Equal(t, 3, req.Segment)
	assert.Equal(t, 4, req.Tile)
}

func TestReadRequestFail(t *testing.T) {
	buf := bytes.NewBuffer([]byte(`Priority: 1`))
	res, err := model.ReadVideoPacketRequest(bufio.NewReader(buf))

	assert.Nil(t, res)
	assert.NotNil(t, err)
}

func TestWriteResponse(t *testing.T) {
	buf := &bytes.Buffer{}
	(&model.VideoPacketResponse{
		Priority: 1,
		Bitrate:  2,
		Segment:  3,
		Tile:     4,
		Data:     []byte{0x00, 0x01, 0x02},
	}).Write(buf)
	expected := append([]byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4
Content-Length: 3

`), 0x00, 0x01, 0x02)

	assert.Equal(t, expected, buf.Bytes())
}

func TestReadResponse(t *testing.T) {
	buf := bytes.NewBuffer(append([]byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4
Content-Length: 3

`), 0x00, 0x01, 0x02))
	res, err := model.ReadVideoPacketResponse(bufio.NewReader(buf))

	assert.Nil(t, err)
	assert.NotNil(t, res)

	assert.Equal(t, 1, int(res.Priority))
	assert.Equal(t, 2, int(res.Bitrate))
	assert.Equal(t, 3, res.Segment)
	assert.Equal(t, 4, res.Tile)
	assert.Equal(t, []byte{0x00, 0x01, 0x02}, res.Data)
}

func TestReadResponseFail(t *testing.T) {
	buf := bytes.NewBuffer([]byte(`Priority: 1`))
	res, err := model.ReadVideoPacketResponse(bufio.NewReader(buf))

	assert.Nil(t, res)
	assert.NotNil(t, err)
}
