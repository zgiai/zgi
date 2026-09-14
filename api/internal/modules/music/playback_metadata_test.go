package music

import (
	"reflect"
	"testing"
)

func TestNormalizeMusicWaveformPeaks(t *testing.T) {
	got := normalizeMusicWaveformPeaks([]int{0, 100, 50})
	want := []int16{0, 100, 50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMusicWaveformPeaks() = %#v, want %#v", got, want)
	}
}

func TestBuildMusicWaveformPeaksFromSamples(t *testing.T) {
	got := buildMusicWaveformPeaksFromSamples([]int{1, 4, 2, 8, 3, 6}, 3)
	want := []int{4, 8, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMusicWaveformPeaksFromSamples() = %#v, want %#v", got, want)
	}
}

func TestAbsPCM16HandlesMinimumSample(t *testing.T) {
	if got, want := absPCM16(-32768), 32768; got != want {
		t.Fatalf("absPCM16(-32768) = %d, want %d", got, want)
	}
}

func TestNormalizeMusicBackfillTaskIDsTrimsSplitsAndDeduplicates(t *testing.T) {
	got := normalizeMusicBackfillTaskIDs([]string{" id-1,id-2 ", "id-1", "", " id-3 "})
	want := []string{"id-1", "id-2", "id-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMusicBackfillTaskIDs() = %#v, want %#v", got, want)
	}
}
