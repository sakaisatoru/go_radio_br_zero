package main

import (
	"sync"
	"time"
)

const (
	displayColon string = " :"
)

var (
	displayWeekday = [...]string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
)

type InfomationDisplay struct {
	mu       sync.Mutex
	buff     []byte
	buffPos  int
	buffLen  int
	isScroll bool // 自動スクロール（デフォルトで有効）
}

func InfomationDisplayNew() *InfomationDisplay {
	return &InfomationDisplay{
		isScroll: true,
		buffPos:  0,
	}
}

// Update 指定した行の表示内容を更新する。1行目に指定した場合はバッファリングされる。
func (v *InfomationDisplay) Update(line int, s string) {
	mu.Lock()
	defer mu.Unlock()

	t := []byte(s)
	l := lcd.UTF8toOLED(&t)
	if l > 8 {
		t = append(t[:l], append([]byte("  "), t[:8]...)...)
	} else {
		t = append(t[:l], []byte("        ")...)[:8]
	}
	if line == 0 {
		if l > 8 && !v.isScroll {
			v.buff = t[:8]
		} else {
			v.buff = t
		}
		v.buffLen = len(v.buff)
		v.buffPos = 0
	}
	lcd.PrintWithPos(0, uint8(line), t[:8])
}

// ShowError エラーメッセージを表示する。
func (v *InfomationDisplay) ShowError(e int) {
	v.buff = []byte(errmessage[e])
	v.buffLen = len(v.buff)
	lcd.PrintWithPos(0, 0, v.buff[:8])
}

// ShowClock 時計を表示する。バッファされている文字列があれば1行目に表示する。
func (v *InfomationDisplay) ShowClock(alarmflags string) {
	var c, tm, dt string
	mu.Lock()
	defer mu.Unlock()

	n := time.Now().In(jst) //Local()
	if colon == 0 {
		c = " "
	} else {
		c = ":"
	}

	tm = alarmflags + " " + n.Format("15") + c + n.Format("04")
	lcd.PrintWithPos(0, 1, []byte(tm))

	if !radioState.IsRadioEnable() {
		//~ dt := fmt.Sprintf("%02d-%02d %2.2s", n.Month(), n.Day(), n.Weekday())
		dt = n.Format("01-02") + " " + displayWeekday[n.Weekday()]
		lcd.PrintWithPos(0, 0, []byte(dt))
		return
	}

	if v.buffLen <= 8 || !v.isScroll {
		lcd.PrintWithPos(0, 0, v.buff)
	} else {
		lcd.PrintWithPos(0, 0, v.buff[v.buffPos:v.buffPos+8])
		v.buffPos++
		if v.buffPos >= v.buffLen-8 {
			v.buffPos = 0
		}
	}
}

// isScroll 1行目のスクロールするかどうかを返す。
func (v *InfomationDisplay) IsScroll() bool {
	return v.isScroll
}

// Scroll 1行目をスクロールさせる。（デフォルト）
func (v *InfomationDisplay) Scroll() {
	v.isScroll = true
}

// Fix 1行目をスクロールしない。
func (v *InfomationDisplay) Fix() {
	v.isScroll = false
}
