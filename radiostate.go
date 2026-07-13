package main

import (
	"github.com/sakaisatoru/go_mpvradio/netradio"
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
	"local.packages/volume"
	"time"
)

type StateCode int

const (
	stateNormalMode     StateCode = iota // radio off
	stateVolumeSet                       // 音量調整
	stateTuneMode                        // 選局
	stateSelectFunction                  // アラームON -> スリープON -> アラーム・スリープON -> ALL OFF
	stateAlarmHourSet                    // アラーム時セット
	stateAlarmMinSet                     // アラーム分セット
)

type TokeiState int

const (
	tokeiNormal TokeiState = iota
	tokeiAlarmOn
	tokeiSleepOn
)

const (
	stationRestoreDuration time.Duration = 5000 * time.Millisecond
)

type RadioState struct {
	Led
	currState      StateCode
	AlarmTime      time.Time
	TurnOffTime    time.Time
	radioEnable    bool
	pos            int
	lastpos        int
	stationList    []netradio.StationInfo
	stationListLen int
	tokeiState     TokeiState
	restoreTimer   *time.Timer
}

func RadioStateNew() *RadioState {
	v := &RadioState{
		currState:      stateNormalMode,
		AlarmTime:      time.Date(2026, time.July, 5, 4, 50, 0, 0, time.UTC),
		TurnOffTime:    time.Unix(0, 0).UTC(),
		radioEnable:    false,
		pos:            0,
		stationListLen: 0,
		tokeiState:     tokeiNormal,
	}

	// 選局中に一定時間確定しなかったら元の局を表示する
	v.restoreTimer = time.AfterFunc(stationRestoreDuration, func() {
		v.pos = v.lastpos
		infomation.Update(0, v.CurrentStationName())
	})
	v.restoreTimer.Stop()
	return v
}

// GetTokeiState アラームやスリープの設定状況を文字列で返す。
func (v *RadioState) GetTokeiState() string {
	var a, s string
	if (v.tokeiState & tokeiAlarmOn) != 0 {
		a = "A"
	} else {
		a = " "
	}
	if (v.tokeiState & tokeiSleepOn) != 0 {
		s = "S"
	} else {
		s = " "
	}
	return a + s
}

// ChannelUpdate 現在の局を保存する
func (v *RadioState) CannelUpdate() {
	v.lastpos = v.pos
}

// IsCannelChange 選局が変更されたかを返す
func (v *RadioState) IsCannelChange() bool {
	return (v.lastpos != v.pos)
}

// GetStateString 現在の状態を文字列で返す
func (v *RadioState) GetStateString(c uint8) string {
	var h, m, flags string

	flags = v.GetTokeiState()
	switch v.currState {
	case stateNormalMode, stateVolumeSet, stateTuneMode:
		return flags

	case stateSelectFunction, stateAlarmHourSet, stateAlarmMinSet:
		h = v.AlarmTime.Format("15") // hour
		m = v.AlarmTime.Format("04") // minute

		if c == 0 {
			switch v.currState {
			case stateAlarmHourSet:
				// blink Hour
				h = "  "
			case stateAlarmMinSet:
				// blink Min
				m = "  "
			}
		}

		return flags + " " + h + ":" + m
	}
	return ""
}

// TokeiCheck アラームおよびスリープ時刻をチェックしてそれぞれを起動する
func (v *RadioState) TokeiCheck() {
	if (v.currState != stateAlarmHourSet) && (v.currState != stateAlarmMinSet) {
		if (v.tokeiState & tokeiAlarmOn) != 0 {
			// アラーム
			n := time.Now()
			if v.AlarmTime.Hour() == n.Hour() &&
				v.AlarmTime.Minute() == n.Minute() {
				v.tokeiState ^= tokeiAlarmOn
				tune()
			}
		}
		if (v.tokeiState & tokeiSleepOn) != 0 {
			// スリープ
			n := time.Now()
			if v.TurnOffTime.Hour() == n.Hour() &&
				v.TurnOffTime.Minute() == n.Minute() {
				v.tokeiState ^= tokeiSleepOn
				mpvctl.Stop()
			}
		}
	}
}

// ReadStationListInfo 放送局のリストを設定する
func (v *RadioState) ReadStationListInfo(s string) error {
	var err error
	v.stationList, err = netradio.PrepareStationList(s, 8)
	if err != nil {
		return err
	}
	v.stationListLen = len(v.stationList)
	return nil
}

// CurrentStationName 現在受信中の局名を返す
func (v *RadioState) CurrentStationName() string {
	return v.stationList[v.pos].Name
}

// CurrentStationURL 現在受信中の局のURLを返す
func (v *RadioState) CurrentStationURL() string {
	return v.stationList[v.pos].Url
}

// CurrentStationIndex 現在受信中の局の局情報の格納スライスの添字を返す
func (v *RadioState) CurrentStationIndex() int {
	return v.pos
}

// RadioEnable 受信状態を設定する
func (v *RadioState) RadioEnable() {
	v.radioEnable = true
}

// RadioDisable 受信していない状態を設定する
func (v *RadioState) RadioDisable() {
	v.radioEnable = false
}

// IsRadioEnable 受信状態を返す
func (v *RadioState) IsRadioEnable() bool {
	return v.radioEnable
}

// GetState 現在の動作状態を返す
func (v *RadioState) GetState() StateCode {
	return v.currState
}

// AlarmTimeInc アラーム時刻を進める
func (v *RadioState) AlarmTimeInc() {
	if v.currState == stateAlarmHourSet {
		v.AlarmTime = v.AlarmTime.Add(1 * time.Hour)
	} else {
		v.AlarmTime = v.AlarmTime.Add(1 * time.Minute)
	}
}

// AlarmTimeDec アラーム時刻を戻す
func (v *RadioState) AlarmTimeDec() {
	if v.currState == stateAlarmMinSet {
		v.AlarmTime = v.AlarmTime.Add(59 * time.Minute)
		// 時間が進んでしまうのでhourも補正する
	}
	v.AlarmTime = v.AlarmTime.Add(23 * time.Hour)
}

// nextTune 選局
func (v *RadioState) NextTune() {
	if v.radioEnable {
		if v.pos < v.stationListLen-1 {
			v.pos++
		}
	}
}

// priorTune 選局
func (v *RadioState) PriorTune() {
	if v.radioEnable {
		if v.pos > 0 {
			v.pos--
		}
	}
}

// TransitionState 遷移時に一度だけ実行される動作
func (v *RadioState) TransitionState(s StateCode) {
	// 現在のモードの後始末
	//~ switch v.currState {
	//~ case stateTuneMode:
	//~ infomation.Scroll()
	//~ }

	// 新しく遷移するモードの初期化処理
	switch s {
	case stateNormalMode:
		if v.radioEnable {
			// ラジオが鳴っていれば入力待ちからボリューム操作へ遷移する
			s = stateVolumeSet
		}
	case stateTuneMode:
		//~ infomation.Fix()
	}

	v.currState = s
	v.ChangeColor(s)
}

// 各モードにおけるボタンへの機能割当

// handleNormalMode ホームポジション
func (v *RadioState) handleNormalMode(btn ButtonCode) {
	switch btn {
	case BtnStationReForward, BtnStationReButton:
		tune()
		v.TransitionState(stateVolumeSet)
	case BtnStationReBackward:
		// （空きファンクション）
	case BtnStationReButtonLong:
		// （空きファンクション）
	case BtnStationReButtonRepeat:
		// （空きファンクション）
	}
}

// handleAlarmHourSet アラームセット（時）
func (v *RadioState) handleAlarmHourSet(btn ButtonCode) {
	switch btn {
	case BtnStationReForward:
		v.AlarmTimeInc()
		lcd.PrintWithPos(0, uint8(1), []byte(v.GetStateString(1)))
	case BtnStationReBackward:
		v.AlarmTimeDec()
		lcd.PrintWithPos(0, uint8(1), []byte(v.GetStateString(1)))
	case BtnStationReButton:
		v.TransitionState(stateAlarmMinSet)
	case BtnStationReButtonLong:
		v.TransitionState(stateNormalMode)
	}
}

// handleAlarmMinSet アラームセット（分）
func (v *RadioState) handleAlarmMinSet(btn ButtonCode) {
	switch btn {
	case BtnStationReForward:
		v.AlarmTimeInc()
		lcd.PrintWithPos(0, uint8(1), []byte(v.GetStateString(1)))
	case BtnStationReBackward:
		v.AlarmTimeDec()
		lcd.PrintWithPos(0, uint8(1), []byte(v.GetStateString(1)))
	case BtnStationReButton:
		v.TransitionState(stateSelectFunction)
	case BtnStationReButtonLong:
		v.TransitionState(stateNormalMode)
	}
}

// handleSelectFunction アラームやスリープの設定
func (v *RadioState) handleSelectFunction(btn ButtonCode) {
	switch btn {
	case BtnStationReButton:
		if v.tokeiState == 3 {
			v.tokeiState = 0
			v.TransitionState(stateAlarmHourSet)
			break
		}
		v.tokeiState++
		v.tokeiState &= 3
		if (v.tokeiState & tokeiSleepOn) != 0 {
			// スリープ時刻の設定を行う
			v.TurnOffTime = time.Now().Add(30 * time.Minute)
		}
	case BtnStationReButtonLong:
		v.TransitionState(stateNormalMode)
	}
}

// handleTuneMode 選局
func (v *RadioState) handleTuneMode(btn ButtonCode) {
	v.restoreTimer.Stop()
	switch btn {
	case BtnStationReForward:
		radioState.NextTune()
		infomation.Update(0, v.CurrentStationName())
		v.restoreTimer.Reset(stationRestoreDuration)
	case BtnStationReBackward:
		radioState.PriorTune()
		infomation.Update(0, v.CurrentStationName())
		v.restoreTimer.Reset(stationRestoreDuration)
	case BtnStationReButton:
		tune()
		v.TransitionState(stateVolumeSet)
	case BtnStationReButtonLong:
		v.TransitionState(stateSelectFunction)
	}
}

// handleVolumeSet 音量調整
func (v *RadioState) handleVolumeSet(btn ButtonCode) {
	switch btn {
	case BtnStationReForward:
		if !radioState.IsRadioEnable() {
			// 右回転でラジオのスイッチを入れる
			tune()
		}
		volume.Increment()
	case BtnStationReBackward:
		if volume.Get() == mpvctl.VolumeMin {
			// 左に回しきった状態ならラジオを止める
			mpvctl.Stop()
			v.TransitionState(stateNormalMode)
		} else {
			volume.Decrement()
		}
	case BtnStationReButton:
		v.TransitionState(stateTuneMode)
	case BtnStationReButtonLong:
		// radio off
		mpvctl.Stop()
		v.TransitionState(stateNormalMode)
	}
}

// handleGlovalEvent モードに関係なく優先的に実行される可能性のある処理
func (v *RadioState) handleGlovalEvent(btn ButtonCode) bool {
	if v.currState == stateNormalMode && btn == BtnStationReButtonLong {
		shutdown()
		return true
	}
	return false
}

// Dispatch 処理の切り替えを行う ループを継続する場合は false、中断する場合は true を返す
func (v *RadioState) Dispatch(btn ButtonCode) bool {
	if v.handleGlovalEvent(btn) {
		// 優先処理が実行されていれば終わる
		return true
	}

	switch v.currState {
	case stateNormalMode:
		v.handleNormalMode(btn)
	case stateVolumeSet:
		v.handleVolumeSet(btn)
	case stateTuneMode:
		v.handleTuneMode(btn)
	case stateSelectFunction:
		v.handleSelectFunction(btn)
	case stateAlarmHourSet:
		v.handleAlarmHourSet(btn)
	case stateAlarmMinSet:
		v.handleAlarmMinSet(btn)
	}
	return false
}
