package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

var (
	// BytesBOM         = []byte{239, 187, 191}
	layout           = "15:04:05"
	srtTimeSeparator = " --> "
	newLine          = "\n"
	ZeroTime, _      = time.Parse(layout, "00:00:00")
)

// example
// func main() {
// 	file, err := os.Open(subFile)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	srt := NewSrt()
// 	err = srt.ReadSubtitles(file)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// srt.ShiftAll(5500 * time.Millisecond)

// srt.Write(os.Stdout)
// }

// Srt wraps subtitle lines
type Srt struct {
	Subtitles []*Subtitle
}

// Subtitle holds each subtitle text data
type Subtitle struct {
	start time.Time
	end   time.Time
	text  []string
}

// NewReader returns utf-8 compatible Reader
func NewReader(r io.Reader) io.Reader {
	var buff bytes.Buffer
	newReader := io.TeeReader(r, &buff)

	data, err := ioutil.ReadAll(newReader)
	if err != nil {
		log.Fatal(err)
	}
	if utf8.Valid(data) {
		return &buff
	}

	return charmap.Windows1256.NewDecoder().Reader(&buff)
}

// NewSrt returns a Srt
func NewSrt() *Srt {
	return &Srt{
		Subtitles: make([]*Subtitle, 0),
	}
}

// ReadSubtitles loads subtitle lines
func (s *Srt) ReadSubtitles(r io.Reader) error {

	reader := NewReader(r)
	scanner := bufio.NewScanner(reader)

	sub := &Subtitle{}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, srtTimeSeparator) {
			// remove index of subtitle
			sub.text = sub.text[:len(sub.text)-1]

			if len(sub.text) > 0 {
				if len(sub.text[len(sub.text)-1]) == 0 {
					sub.text = sub.text[:len(sub.text)-1]
				}
			}

			start, end, err := parseSubTime(line)
			if err != nil {
				return fmt.Errorf("can't parse srt duration %s, %s", line, err)
			}
			sub = &Subtitle{
				start: start,
				end:   end,
			}

			s.Subtitles = append(s.Subtitles, sub)
			sub.text = make([]string, 0)
		} else {
			sub.text = append(sub.text, line)
		}
	}
	return nil
}

// Write writes formatted subtitles to a writer
func (s *Srt) Write(w io.Writer) error {
	if len(s.Subtitles) == 0 {
		return errors.New("no subtitles to write")
	}

	var buf strings.Builder
	// buf.Write(BytesBOM)

	for i, sub := range s.Subtitles {
		buf.WriteString(fmt.Sprintf("%d", i+1) + newLine)
		buf.WriteString(formatSubTime(sub.start, sub.end) + newLine)
		for _, line := range sub.text {
			buf.WriteString(line + newLine)
		}
		buf.WriteString(newLine)
	}

	if _, err := w.Write([]byte(buf.String())); err != nil {
		return err
	}

	return nil
}

// parseSubTime reads subtitle time to go time
func parseSubTime(str string) (start, end time.Time, err error) {
	var h1, m1, s1, ms1, h2, m2, s2, ms2 time.Duration
	start, end = ZeroTime, ZeroTime

	_, err = fmt.Sscanf(str, "%d:%d:%d,%d --> %d:%d:%d,%d",
		&h1, &m1, &s1, &ms1,
		&h2, &m2, &s2, &ms2)
	if err != nil {
		return
	}

	start = start.Add(h1*time.Hour + m1*time.Minute + s1*time.Second + ms1*time.Millisecond)
	end = end.Add(h2*time.Hour + m2*time.Minute + s2*time.Second + ms2*time.Millisecond)

	return
}

// formatSubTime formats given start/end times to subtitle time format
func formatSubTime(start time.Time, end time.Time) string {
	if start.Before(ZeroTime) {
		start = ZeroTime
	}

	return fmt.Sprintf("%02d:%02d:%02d,%03d --> %02d:%02d:%02d,%03d",
		start.Hour(), start.Minute(), start.Second(), start.Nanosecond()/1000/1000,
		end.Hour(), end.Minute(), end.Second(), end.Nanosecond()/1000/1000,
	)
}

// ShiftAll shifts all subtitles by given duration
// e.g.   move all subtitles +2 seconds forward
func (s *Srt) ShiftAll(dur time.Duration) {
	for _, sub := range s.Subtitles {
		sub.Shift(dur)
	}
}

// ShiftPart shifts subtitles between given start and end time
// e.g.   move subtitles between start and end by -2 second
func (s *Srt) ShiftPart(start, end time.Time, dur time.Duration) {
	for _, sub := range s.Subtitles {
		if sub.start.After(start) && sub.end.Before(end) {
			sub.Shift(dur)
		}
	}
}

// Shift shifts subtitle time by duration
// e.g. -2/+2 seconds
func (sub *Subtitle) Shift(dur time.Duration) {
	if sub.start.Add(dur).After(ZeroTime) {
		sub.start = sub.start.Add(dur)
	}
	sub.end = sub.end.Add(dur)
}

// ShiftStart shifts only start time of subtitle by duration
func (sub *Subtitle) ShiftStart(dur time.Duration) {
	if sub.start.Add(dur).After(ZeroTime) {
		sub.start = sub.start.Add(dur)
	}
}

// ShiftStart shifts only end time of subtitle by duration
func (sub *Subtitle) ShiftEnd(dur time.Duration) {
	sub.end = sub.end.Add(dur)
}

// CutPart removes subtitles between start time and end time
func (s *Srt) CutPart(start, end time.Time) {
	newSubs := make([]*Subtitle, 0)
	dur := end.Sub(start)

	for _, sub := range s.Subtitles {
		if sub.start.After(start) && sub.end.Before(end) {
			continue
		}
		if sub.end.After(end) {
			sub.Shift(dur)
		}
		newSubs = append(newSubs, sub)
	}

	s.Subtitles = newSubs
}

// ShiftSync shifts subtitles relatively by duration
// given 20 seconds means it shifts zero seconds to
// first subtitle and 20 seconds to last one. And all
// subtitles between shift relatively to the whole
// duration of file.
func (s *Srt) ShiftSync(changeDur time.Duration) {
	lastSub := s.Subtitles[len(s.Subtitles)-1]
	totalDur := lastSub.end.Sub(ZeroTime)
	totalDurMil := totalDur.Nanoseconds() / 1000 / 1000
	changeDurMil := changeDur.Nanoseconds() / 1000 / 1000

	for _, sub := range s.Subtitles {
		startDurMil := sub.start.Sub(ZeroTime).Nanoseconds() / 1000 / 1000
		startDiff := float64(startDurMil) / float64(totalDurMil) * float64(changeDurMil)
		durStart := time.Duration(startDiff) * time.Millisecond

		endDurMil := sub.end.Sub(ZeroTime).Nanoseconds() / 1000 / 1000
		endDiff := float64(endDurMil) / float64(totalDurMil) * float64(changeDurMil)
		durEnd := time.Duration(endDiff) * time.Millisecond

		sub.ShiftStart(durStart)
		sub.ShiftEnd(durEnd)
	}
}

// StripTags removes html tags from subtitle
func (s *Srt) StripTags() error {
	// implement stripeTags to remove html tags
	return nil
}
