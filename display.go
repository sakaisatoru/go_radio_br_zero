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

	r, l := lcd.UTF8toOLED(s)
	t := make([]byte, 0, l + 2 + 8)
	t = append(t, r[:l]...)
	if l > 8 {
		t = append(t, "  "...)
		t = append(t, r[:8]...)
	} else {
		t = append(t, "        "...)
	}
	if line == 0 {
		if l <= 8 {
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
	var c, dt string

	mu.Lock()
	defer mu.Unlock()

	if len(alarmflags) > 2 {
		// フラグ以外のもの（アラーム時刻等）が含まれていればそのまま表示して終わる。
		lcd.PrintWithPos(0, 1, []byte(alarmflags))
		return
	}

	n := time.Now().In(jst) //Local()
	if colon == 0 {
		c = " "
	} else {
		c = ":"
	}

	alarmflags = alarmflags + " " + n.Format("15") + c + n.Format("04")
	lcd.PrintWithPos(0, 1, []byte(alarmflags))

	if !radioState.IsRadioEnable() {
		// ラジオが切られていたら日付を表示して終わる
		dt = n.Format("01-02") + " " + displayWeekday[n.Weekday()]
		lcd.PrintWithPos(0, 0, []byte(dt))
		return
	}

	if v.isScroll && v.buffLen > 8 {
		lcd.PrintWithPos(0, 0, v.buff[v.buffPos:v.buffPos+8])
		v.buffPos++
		if v.buffPos >= v.buffLen - 8 {
			v.buffPos = 0
		}
		return
	}

	l := 0
	if v.buffLen > 8 {
		l = 8
	} else {
		l = v.buffLen
	}
	lcd.PrintWithPos(0, 0, v.buff[:l])
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
