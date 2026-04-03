package metrics

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"main/src/model"
)

type StatisticsLogger struct {
	fileWriter *bufio.Writer
	mutex      sync.Mutex
	file       fs.File
}

func NewStatisticsLogger(path string) *StatisticsLogger {
	const header = "time_ns,segment,tile,priority,latency_ns,timedout,skipped,ok,tp,buffer_s,tile_missing_ratio,in_fov,on_time\n"
	file, err := os.Create(path)
	if err != nil {
		log.Panicf("Failed to open %s: %s\n", path, err)
	}
	fileWriter := bufio.NewWriter(file)
	if _, err := fileWriter.WriteString(header); err != nil {
		log.Panicf("Failed to write to %s: %s\n", path, err)
	}
	return &StatisticsLogger{fileWriter: fileWriter, file: file}
}

func (s *StatisticsLogger) Log(timeFromStart time.Duration, r model.VideoPacketRequest, latency time.Duration, timedOut bool, skipped bool, ok bool, tp float64, bufferSec float64, tileMissingRatio float64, inFOV bool, onTime bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	row := fmt.Sprintf("%d,%d,%d,%d,%d,%t,%t,%t,%f,%.2f,%.2f,%t,%t\n", timeFromStart.Nanoseconds(), r.Segment, r.Tile, r.Priority, latency.Nanoseconds(), timedOut, skipped, ok, tp, bufferSec, tileMissingRatio, inFOV, onTime)
	if _, err := s.fileWriter.WriteString(row); err != nil {
		log.Panicf("Failed to write: %s\n", err)
	}
}

func (s *StatisticsLogger) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.fileWriter.Flush()
	s.file.Close()
}

type SummaryLogger struct {
	fileWriter *bufio.Writer
	mutex      sync.Mutex
	file       fs.File
}

func NewSummaryLogger(path string) *SummaryLogger {
	const header = "join_latency_ms,segment_completion_rate_percent,segment_completion_rate_fov_percent,stale_bytes_ratio_percent,deadline_miss_rate_fov_percent,deadline_miss_rate_nonfov_percent,fov_hit_rate_delivery_percent,useful_goodput_fov_kbps,timely_bytes_ratio_percent,client_quic_uplink_loss_rate_percent,client_quic_uplink_lost_packets,client_quic_uplink_acked_packets\n"
	file, err := os.Create(path)
	if err != nil {
		log.Panicf("Failed to open %s: %s\n", path, err)
	}
	fileWriter := bufio.NewWriter(file)
	if _, err := fileWriter.WriteString(header); err != nil {
		log.Panicf("Failed to write to %s: %s\n", path, err)
	}
	return &SummaryLogger{fileWriter: fileWriter, file: file}
}

func (s *SummaryLogger) LogSession(joinLatency time.Duration, segmentCompletionRatePercent float64, fovCompletionRatePercent float64, staleBytesRatioPercent float64, deadlineMissRateFOV float64, deadlineMissRateNonFOV float64, fovHitRate float64, usefulGoodputKbps float64, timelyBytesRatioPercent float64, clientQUICUplinkLossRatePercent float64, clientQUICUplinkLostPackets uint64, clientQUICUplinkAckedPackets uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	row := fmt.Sprintf("%d,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%.2f,%d,%d\n", joinLatency.Milliseconds(), segmentCompletionRatePercent, fovCompletionRatePercent, staleBytesRatioPercent, deadlineMissRateFOV, deadlineMissRateNonFOV, fovHitRate, usefulGoodputKbps, timelyBytesRatioPercent, clientQUICUplinkLossRatePercent, clientQUICUplinkLostPackets, clientQUICUplinkAckedPackets)
	if _, err := s.fileWriter.WriteString(row); err != nil {
		log.Panicf("Failed to write: %s\n", err)
	}
}

func (s *SummaryLogger) Close() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.fileWriter.Flush()
	s.file.Close()
}

type staleBytesAgg struct {
	mutex      sync.Mutex
	lateBytes  uint64
	totalBytes uint64
}

func (a *staleBytesAgg) Add(bytes int, late bool) {
	if bytes <= 0 {
		return
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.totalBytes += uint64(bytes)
	if late {
		a.lateBytes += uint64(bytes)
	}
}
func (a *staleBytesAgg) RatioPercent() float64 {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.totalBytes == 0 {
		return 0
	}
	return 100.0 * float64(a.lateBytes) / float64(a.totalBytes)
}
func (a *staleBytesAgg) TimelyPercent() float64 {
	return 100.0 - a.RatioPercent()
}

type tileDeadlineMissAgg struct {
	mutex       sync.Mutex
	totalFOV    uint64
	missFOV     uint64
	totalNonFOV uint64
	missNonFOV  uint64
}

func (a *tileDeadlineMissAgg) Add(isFOV bool, missed bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if isFOV {
		a.totalFOV++
		if missed {
			a.missFOV++
		}
	} else {
		a.totalNonFOV++
		if missed {
			a.missNonFOV++
		}
	}
}
func (a *tileDeadlineMissAgg) Rates() (float64, float64) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	fovRate := 0.0
	nonFovRate := 0.0
	if a.totalFOV > 0 {
		fovRate = 100.0 * float64(a.missFOV) / float64(a.totalFOV)
	}
	if a.totalNonFOV > 0 {
		nonFovRate = 100.0 * float64(a.missNonFOV) / float64(a.totalNonFOV)
	}
	return fovRate, nonFovRate
}

type fovHitSample struct {
	Segment int
	Total   uint64
	OnTime  uint64
	Rate    float64
}

type fovHitAgg struct {
	mutex  sync.Mutex
	total  map[int]uint64
	onTime map[int]uint64
}

func newFovHitAgg() *fovHitAgg {
	return &fovHitAgg{total: make(map[int]uint64), onTime: make(map[int]uint64)}
}
func (a *fovHitAgg) Add(segment int, inFOV bool, onTime bool) {
	if !inFOV || segment <= 0 {
		return
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.total[segment]++
	if onTime {
		a.onTime[segment]++
	}
}
func (a *fovHitAgg) RateOverall() float64 {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	var total uint64
	var hit uint64
	for seg, cnt := range a.total {
		total += cnt
		hit += a.onTime[seg]
	}
	if total == 0 {
		return 0
	}
	return 100.0 * float64(hit) / float64(total)
}
func (a *fovHitAgg) Series(firstSegment, lastSegment int) []fovHitSample {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if lastSegment < firstSegment {
		return nil
	}
	series := make([]fovHitSample, 0, lastSegment-firstSegment+1)
	for seg := firstSegment; seg <= lastSegment; seg++ {
		total := a.total[seg]
		if total == 0 {
			continue
		}
		onTime := a.onTime[seg]
		rate := 100.0 * float64(onTime) / float64(total)
		series = append(series, fovHitSample{Segment: seg, Total: total, OnTime: onTime, Rate: rate})
	}
	return series
}

type fovGoodputSample struct {
	WindowStart time.Duration
	WindowEnd   time.Duration
	Bytes       uint64
	Kbps        float64
}

type ClientQUICUplinkLossRateSample struct {
	WindowStart  time.Duration
	WindowEnd    time.Duration
	LostPackets  uint64
	AckedPackets uint64
	Percent      float64
}

type clientQUICUplinkLossRateBucket struct {
	lost  uint64
	acked uint64
}

type ClientQUICUplinkLossRateAgg struct {
	mutex      sync.Mutex
	window     time.Duration
	startTime  time.Time
	active     bool
	totalLost  uint64
	totalAcked uint64
	buckets    map[int64]clientQUICUplinkLossRateBucket
	now        func() time.Time
}

func NewClientQUICUplinkLossRateAgg(window time.Duration) *ClientQUICUplinkLossRateAgg {
	if window <= 0 {
		window = time.Second
	}
	return &ClientQUICUplinkLossRateAgg{
		window:  window,
		buckets: make(map[int64]clientQUICUplinkLossRateBucket),
		now:     time.Now,
	}
}

func (a *ClientQUICUplinkLossRateAgg) StartDataPhase(start time.Time) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if start.IsZero() {
		start = a.now()
	}
	a.startTime = start
	a.active = true
}

func (a *ClientQUICUplinkLossRateAgg) AddLost(at time.Time) {
	a.add(at, true)
}

func (a *ClientQUICUplinkLossRateAgg) AddAcked(at time.Time) {
	a.add(at, false)
}

func (a *ClientQUICUplinkLossRateAgg) add(at time.Time, lost bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if !a.active {
		return
	}
	if at.IsZero() {
		at = a.now()
	}
	if a.startTime.IsZero() {
		a.startTime = at
	}
	if at.Before(a.startTime) {
		return
	}
	bucketIdx := int64(0)
	if a.window > 0 {
		bucketIdx = int64(at.Sub(a.startTime) / a.window)
	}
	bucket := a.buckets[bucketIdx]
	if lost {
		a.totalLost++
		bucket.lost++
	} else {
		a.totalAcked++
		bucket.acked++
	}
	a.buckets[bucketIdx] = bucket
}

func (a *ClientQUICUplinkLossRateAgg) OverallPercent() float64 {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return lossRatePercent(a.totalLost, a.totalAcked)
}

func (a *ClientQUICUplinkLossRateAgg) Totals() (uint64, uint64) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return a.totalLost, a.totalAcked
}

func (a *ClientQUICUplinkLossRateAgg) Series() []ClientQUICUplinkLossRateSample {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if len(a.buckets) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(a.buckets))
	for k := range a.buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	samples := make([]ClientQUICUplinkLossRateSample, 0, len(keys))
	for _, bucketIdx := range keys {
		bucket := a.buckets[bucketIdx]
		start := time.Duration(bucketIdx) * a.window
		end := start + a.window
		samples = append(samples, ClientQUICUplinkLossRateSample{
			WindowStart:  start,
			WindowEnd:    end,
			LostPackets:  bucket.lost,
			AckedPackets: bucket.acked,
			Percent:      lossRatePercent(bucket.lost, bucket.acked),
		})
	}
	return samples
}

func lossRatePercent(lost uint64, acked uint64) float64 {
	resolved := lost + acked
	if resolved == 0 {
		return 0
	}
	return 100.0 * float64(lost) / float64(resolved)
}

type fovGoodputAgg struct {
	mutex      sync.Mutex
	window     time.Duration
	totalBytes uint64
	buckets    map[int64]uint64
}

func newFovGoodputAgg(window time.Duration) *fovGoodputAgg {
	return &fovGoodputAgg{window: window, buckets: make(map[int64]uint64)}
}
func (a *fovGoodputAgg) Add(at time.Duration, bytes int, inFOV bool, onTime bool) {
	if !inFOV || !onTime || bytes <= 0 {
		return
	}
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.totalBytes += uint64(bytes)
	var bucket int64
	if a.window > 0 {
		bucket = int64(at / a.window)
	}
	a.buckets[bucket] += uint64(bytes)
}
func (a *fovGoodputAgg) OverallKbps(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	a.mutex.Lock()
	total := a.totalBytes
	a.mutex.Unlock()
	return (8.0 * float64(total)) / (elapsed.Seconds() * 1000.0)
}
func (a *fovGoodputAgg) Series() []fovGoodputSample {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if len(a.buckets) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(a.buckets))
	for k := range a.buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	samples := make([]fovGoodputSample, 0, len(keys))
	for _, bucket := range keys {
		bytes := a.buckets[bucket]
		start := time.Duration(bucket) * a.window
		end := start + a.window
		kbps := (8.0 * float64(bytes)) / (a.window.Seconds() * 1000.0)
		samples = append(samples, fovGoodputSample{WindowStart: start, WindowEnd: end, Bytes: bytes, Kbps: kbps})
	}
	return samples
}

type segmentCompletionAgg struct {
	required  map[int]map[int]struct{}
	ontime    map[int]map[int]struct{}
	processed map[int]map[int]struct{}
	mutex     sync.Mutex
}

func newSegmentCompletionAgg() *segmentCompletionAgg {
	return &segmentCompletionAgg{required: make(map[int]map[int]struct{}), ontime: make(map[int]map[int]struct{}), processed: make(map[int]map[int]struct{})}
}
func (a *segmentCompletionAgg) SetRequired(segment int, tiles []int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	s := make(map[int]struct{}, len(tiles))
	for _, t := range tiles {
		s[t] = struct{}{}
	}
	a.required[segment] = s
	a.ontime[segment] = make(map[int]struct{})
	a.processed[segment] = make(map[int]struct{})
}
func (a *segmentCompletionAgg) Record(segment, tile int, onTime bool) (float64, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	req, ok := a.required[segment]
	if !ok {
		return -1.0, false
	}
	proc := a.processed[segment]
	if _, already := proc[tile]; !already {
		proc[tile] = struct{}{}
		if onTime {
			a.ontime[segment][tile] = struct{}{}
		}
	}
	if len(proc) == len(req) && len(req) > 0 {
		onTimeCount := len(a.ontime[segment])
		missing := len(req) - onTimeCount
		return float64(missing) / float64(len(req)), true
	}
	return -1.0, false
}
func (a *segmentCompletionAgg) Rate(firstSegment, lastSegment int) float64 {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	total := 0
	completed := 0
	for seg := firstSegment; seg <= lastSegment; seg++ {
		req := a.required[seg]
		if len(req) == 0 {
			continue
		}
		total++
		if len(a.ontime[seg]) == len(req) {
			completed++
		}
	}
	if total == 0 {
		return 0
	}
	return 100.0 * float64(completed) / float64(total)
}

type deadlineLatenessSample struct {
	Segment        int
	Tile           int
	LatenessMs     float64
	MissedDeadline bool
}
type deadlineLatenessAgg struct {
	mutex    sync.Mutex
	required map[int]map[int]struct{}
	samples  []deadlineLatenessSample
}

func newDeadlineLatenessAgg() *deadlineLatenessAgg {
	return &deadlineLatenessAgg{required: make(map[int]map[int]struct{})}
}
func (a *deadlineLatenessAgg) SetRequired(segment int, tiles []int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	s := make(map[int]struct{}, len(tiles))
	for _, t := range tiles {
		s[t] = struct{}{}
	}
	a.required[segment] = s
}
func (a *deadlineLatenessAgg) Record(segment int, tile int, lateness time.Duration) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	ms := float64(lateness.Microseconds()) / 1000.0
	a.samples = append(a.samples, deadlineLatenessSample{
		Segment:        segment,
		Tile:           tile,
		LatenessMs:     ms,
		MissedDeadline: lateness > 0,
	})
}
func (a *deadlineLatenessAgg) Series(firstSegment, lastSegment int) []deadlineLatenessSample {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	out := make([]deadlineLatenessSample, 0, len(a.samples))
	for _, sample := range a.samples {
		if sample.Segment >= firstSegment && sample.Segment <= lastSegment {
			out = append(out, sample)
		}
	}
	return out
}

type Session struct {
	Stale            *staleBytesAgg
	Deadlines        *tileDeadlineMissAgg
	FOVHit           *fovHitAgg
	FOVGoodput       *fovGoodputAgg
	AllTiles         *segmentCompletionAgg
	FOVTiles         *segmentCompletionAgg
	DeadlineLateness *deadlineLatenessAgg
}

func NewSession(segmentDuration time.Duration) *Session {
	return &Session{
		Stale:            &staleBytesAgg{},
		Deadlines:        &tileDeadlineMissAgg{},
		FOVHit:           newFovHitAgg(),
		FOVGoodput:       newFovGoodputAgg(segmentDuration),
		AllTiles:         newSegmentCompletionAgg(),
		FOVTiles:         newSegmentCompletionAgg(),
		DeadlineLateness: newDeadlineLatenessAgg(),
	}
}

func WriteFOVDeliverySeries(path string, samples []fovHitSample) {
	if path == "" {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to create %s: %v", path, err)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	writer.WriteString("segment,fov_tiles,fov_on_time,fov_hit_rate_percent\n")
	for _, s := range samples {
		fmt.Fprintf(writer, "%d,%d,%d,%.2f\n", s.Segment, s.Total, s.OnTime, s.Rate)
	}
}

func WriteFOVGoodputSeries(path string, samples []fovGoodputSample) {
	if path == "" {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to create %s: %v", path, err)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	writer.WriteString("window_start_s,window_end_s,fov_on_time_bytes,useful_goodput_kbps\n")
	for _, s := range samples {
		fmt.Fprintf(writer, "%.3f,%.3f,%d,%.2f\n", s.WindowStart.Seconds(), s.WindowEnd.Seconds(), s.Bytes, s.Kbps)
	}
}

func WriteDeadlineLatenessSeries(path string, samples []deadlineLatenessSample) {
	if path == "" {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to create %s: %v", path, err)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	writer.WriteString("segment,tile,lateness_ms,missed_deadline\n")
	for _, s := range samples {
		fmt.Fprintf(writer, "%d,%d,%.3f,%t\n", s.Segment, s.Tile, s.LatenessMs, s.MissedDeadline)
	}
}

func WriteClientQUICUplinkLossRateSeries(path string, samples []ClientQUICUplinkLossRateSample) {
	if path == "" {
		return
	}
	file, err := os.Create(path)
	if err != nil {
		log.Printf("Failed to create %s: %v", path, err)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	writer.WriteString("window_start_s,window_end_s,lost_packets,acked_packets,client_quic_uplink_loss_rate_percent\n")
	for _, s := range samples {
		fmt.Fprintf(writer, "%.3f,%.3f,%d,%d,%.2f\n", s.WindowStart.Seconds(), s.WindowEnd.Seconds(), s.LostPackets, s.AckedPackets, s.Percent)
	}
}
