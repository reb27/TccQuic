package stream_handler

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"main/src/model"
)

func TestReadFilePreservesBOLAAndSelectsLegacyLayout(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "segments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bolaData := []byte("historical-bola-layout")
	legacyData := []byte("correct-legacy-layout")
	if err := os.WriteFile(filepath.Join(dir, "video_tiled_5_dash_track7_11.m4s"), bolaData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video_tiled_5_dash_track11_7.m4s"), legacyData, 0o644); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(original)

	bola := readFile(&model.VideoPacketRequest{Bitrate: model.LOW_BITRATE, Segment: 7, Tile: 11})
	if !bytes.Equal(bolaData, bola) {
		t.Fatalf("BOLA lookup changed: got %q", bola)
	}
	legacy := readFile(&model.VideoPacketRequest{Bitrate: model.LOW_BITRATE, Segment: 7, Tile: 11, TileFirstLayout: true})
	if !bytes.Equal(legacyData, legacy) {
		t.Fatalf("Legacy lookup did not select tile-first path: got %q", legacy)
	}
}
