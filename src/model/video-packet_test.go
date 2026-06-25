package model_test

import (
	"bufio"
	"bytes"
	"main/src/model"
	"os"
	"path/filepath"
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
		FoV:      false,
		Timeout:  2000,
	}).Write(buf)
	expected := []byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4
FoV: false
SemanticPriority: 0
Timeout: 2000

`)

	assert.Equal(t, expected, buf.Bytes())
}

func TestWriteAndReadLegacyTileFirstRequest(t *testing.T) {
	buf := &bytes.Buffer{}
	want := &model.VideoPacketRequest{Segment: 3, Tile: 4, TileFirstLayout: true}
	if err := want.Write(buf); err != nil {
		t.Fatal(err)
	}
	assert.Contains(t, buf.String(), "TileFirstLayout: true\n")

	got, err := model.ReadVideoPacketRequest(bufio.NewReader(buf))
	assert.NoError(t, err)
	assert.True(t, got.TileFirstLayout)
}

func TestEstimateTileSizePreservesBOLAAndSelectsLegacyLayout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "segments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSizedFile(t, filepath.Join(dir, "video_tiled_5_dash_track7_11.m4s"), 17)
	writeSizedFile(t, filepath.Join(dir, "video_tiled_5_dash_track11_7.m4s"), 29)
	withWorkingDirectory(t, root, func() {
		bola := &model.VideoPacketRequest{Bitrate: model.LOW_BITRATE, Segment: 7, Tile: 11}
		assert.EqualValues(t, 17, model.EstimateTileSize(bola))
		legacy := &model.VideoPacketRequest{Bitrate: model.LOW_BITRATE, Segment: 7, Tile: 11, TileFirstLayout: true}
		assert.EqualValues(t, 29, model.EstimateTileSize(legacy))
	})
}

func writeSizedFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withWorkingDirectory(t *testing.T, dir string, fn func()) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()
	fn()
}

func TestReadRequest(t *testing.T) {
	buf := bytes.NewBuffer([]byte(`Priority: 1
Bitrate: 2
Segment: 3
Tile: 4
FoV: true
SemanticPriority: 0.6
Timeout: 2000

`))
	req, err := model.ReadVideoPacketRequest(bufio.NewReader(buf))

	assert.NotNil(t, req)
	assert.Nil(t, err)

	assert.Equal(t, 1, int(req.Priority))
	assert.Equal(t, 2, int(req.Bitrate))
	assert.Equal(t, 3, req.Segment)
	assert.Equal(t, 4, req.Tile)
	assert.True(t, req.FoV)
	assert.InDelta(t, 0.6, float64(req.SemanticPriority), 1e-5)
	assert.Equal(t, 2000, req.Timeout)
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
	}).Write(bufio.NewWriter(buf))
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
