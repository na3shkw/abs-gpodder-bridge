package bridge

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule は ABS_INDEX_SCHEDULE で指定する索引の作成時刻。
type Schedule interface {
	// Next は t より後の次の時刻を返す。該当する時刻がなければ false。
	Next(t time.Time) (time.Time, bool)
}

// ParseSchedule は 5 フィールドの cron 式、または "@every {間隔}" を解釈する。
func ParseSchedule(s string) (Schedule, error) {
	if rest, ok := strings.CutPrefix(s, "@every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid interval %q", rest)
		}
		return every(d), nil
	}
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression %q", s)
	}
	var c cronSchedule
	var err error
	if c.minute, _, err = parseCronField(fields[0], 0, 59); err != nil {
		return nil, err
	}
	if c.hour, _, err = parseCronField(fields[1], 0, 23); err != nil {
		return nil, err
	}
	if c.dom, c.domAny, err = parseCronField(fields[2], 1, 31); err != nil {
		return nil, err
	}
	if c.month, _, err = parseCronField(fields[3], 1, 12); err != nil {
		return nil, err
	}
	if c.dow, c.dowAny, err = parseCronField(fields[4], 0, 7); err != nil {
		return nil, err
	}
	if c.dow&(1<<7) != 0 { // 7 も日曜日
		c.dow |= 1
	}
	return &c, nil
}

type every time.Duration

func (e every) Next(t time.Time) (time.Time, bool) { return t.Add(time.Duration(e)), true }

type cronSchedule struct {
	minute, hour, dom, month, dow uint64
	domAny, dowAny                bool
}

func (c *cronSchedule) Next(t time.Time) (time.Time, bool) {
	t = t.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for ; t.Before(limit); t = t.Add(time.Minute) {
		if c.month&(1<<uint(t.Month())) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location()).Add(-time.Minute)
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location()).Add(-time.Minute)
			continue
		}
		if c.hour&(1<<uint(t.Hour())) == 0 {
			t = t.Truncate(time.Hour).Add(time.Hour - time.Minute)
			continue
		}
		if c.minute&(1<<uint(t.Minute())) != 0 {
			return t, true
		}
	}
	return time.Time{}, false
}

// dayMatches は一般的な cron と同じく、日と曜日の両方が指定されていればどちらかが合えばよいとする。
func (c *cronSchedule) dayMatches(t time.Time) bool {
	domOK := c.dom&(1<<uint(t.Day())) != 0
	dowOK := c.dow&(1<<uint(t.Weekday())) != 0
	if c.domAny || c.dowAny {
		return domOK && dowOK
	}
	return domOK || dowOK
}

// parseCronField は "*"、"a"、"a-b"、"*/n"、"a-b/n" とそれらのカンマ区切りを解釈する。
func parseCronField(s string, min, max int) (bits uint64, isAny bool, err error) {
	bad := fmt.Errorf("invalid cron field %q", s)
	for _, part := range strings.Split(s, ",") {
		rng, stepStr, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			if step, err = strconv.Atoi(stepStr); err != nil || step < 1 {
				return 0, false, bad
			}
		}
		lo, hi := min, max
		switch {
		case rng == "*":
			if !hasStep {
				isAny = true
			}
		case strings.Contains(rng, "-"):
			a, b, _ := strings.Cut(rng, "-")
			if lo, err = strconv.Atoi(a); err != nil {
				return 0, false, bad
			}
			if hi, err = strconv.Atoi(b); err != nil {
				return 0, false, bad
			}
		default:
			if lo, err = strconv.Atoi(rng); err != nil {
				return 0, false, bad
			}
			hi = lo
			if hasStep {
				hi = max
			}
		}
		if lo < min || hi > max || lo > hi {
			return 0, false, bad
		}
		for i := lo; i <= hi; i += step {
			bits |= 1 << uint(i)
		}
	}
	return bits, isAny, nil
}
