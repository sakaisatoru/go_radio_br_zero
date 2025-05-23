package main

import (
	"bufio"
	"fmt"
	"github.com/davecheney/i2c"
	"github.com/sakaisatoru/go_radio_raspi/mpvctl"
	"github.com/sakaisatoru/go_radio_raspi/netradio"
	"github.com/sakaisatoru/go_radio_raspi/rotaryencoder"
	"github.com/stianeikeland/go-rpio/v4"
	"local.packages/aqm0802a"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	stationlist     string = "/home/sakai/program/radio.m3u"
	MPV_SOCKET_PATH string = "/run/mpvsocket"
	VERSION         string = "ﾗｼﾞｵv2.5"
)

type ButtonCode int

const (
	btn_station_none ButtonCode = iota
	btn_station_re_button
	btn_station_re_forward
	btn_station_re_backward
	btn_station_repeat_end
	btn_system_shutdown

	btn_station_repeat = 0x80

	btn_press_width      int = 5
	btn_press_long_width int = 25
)

const (
	clock_mode_normal uint8 = iota
	clock_mode_alarm
	clock_mode_sleep
	clock_mode_sleep_alarm
)

const (
	state_radio_off int = iota
	state_volume_controle
	state_station_tuning
	state_select_function
	state_set_alarmtime
	statelength
)

type stateEventhandlers struct {
	cb_click         func()
	cb_re_cw         func()
	cb_re_ccw        func()
	cb_press         func()
	startup          func()
	beforetransition func()
}

const (
	ERROR_HUP = iota
	ERROR_MPV_CONN
	ERROR_MPV_FAULT
	SPACE16
	ERROR_TUNING
	ERROR_RPIO_NOT_OPEN
	ERROR_SOCKET_NOT_OPEN
)

const (
	pin_re_button = 13
	pin_re_a      = 19
	pin_re_b      = 26

	pin_afamp         = 12
	pin_lcd_reset     = 17
	pin_lcd_backlight = 4
	pin_re_led1       = 5
	pin_re_led2       = 6
)

var (
	mpv              net.Conn
	lcd              aqm0802a.AQM0802A
	mu               sync.Mutex
	stlist           []*netradio.StationInfo
	colon            uint8
	pos              int
	lastpos          int
	radio_enable     bool
	volume           int8
	display_colon    = []uint8{' ', ':'}
	display_sleep    = []uint8{' ', ' ', 'S'}
	display_buff     []byte
	display_buff_pos int16 = 0
	clock_mode       uint8
	alarm_time       time.Time
	tuneoff_time     time.Time
	alarm_set_pos    int
	light_timer      *time.Timer

	errmessage = []string{
		"HUP",      // HUP
		"mpv ｴﾗｰ",  //
		"mpv ﾌｫﾙﾄ", //
		"        ", // SPACE16
		"tuneｴﾗｰ",  //
		"rpioｴﾗｰ",  //
		"ｿｹｯﾄｴﾗｰ",  //
	}

	jst       *time.Location
	weekday   = []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	statefunc [statelength]stateEventhandlers
	statepos  int

	voltable = []int8{0, 15, 20, 25, 31, 37, 43, 49, 57, 63, 68}
)

func setup_station_list() int {
	file, err := os.Open(stationlist)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	f := false
	s := ""
	name := ""
	for scanner.Scan() {
		s = scanner.Text()
		if strings.Contains(s, "#EXTINF:") == true {
			f = true
			_, name, _ = strings.Cut(s, "/")
			name = strings.Trim(name, " ")
			continue
		}
		if f {
			if len(s) != 0 {
				f = false
				stmp := new(netradio.StationInfo)
				stmp.Url = s
				if len([]rune(name)) < 8 { // 表示器の桁数で調整すること
					stmp.Name = string([]rune(name + "       ")[:8]) // aqm0802a
				} else {
					stmp.Name = name
				}
				stlist = append(stlist, stmp)
			}
		}
	}
	return len(stlist)
}

func infoupdate(line uint8, mes *string, scroll bool) {
	mu.Lock()
	defer mu.Unlock()

	t := []byte(*mes)
	l := lcd.UTF8toOLED(&t)
	display_buff_pos = 0
	if l > 8 {
		t = append(t[:l], append([]byte("  "), t[:8]...)...)
	} else {
		t = append(t[:l], []byte("        ")...)[:8]
	}
	if line == 0 {
		if l > 8 && scroll == false {
			display_buff = t[:8]
		} else {
			display_buff = t
		}
	}
	lcd.PrintWithPos(0, line, t[:8])
}

func btninput(code chan<- ButtonCode) {
	hold := 0
	btn_h := btn_station_none

	for {
		time.Sleep(10 * time.Millisecond)

		if btn_h == 0 {
			if rpio.Pin(pin_re_button).Read() == rpio.Low {
				// 押されているボタンがあれば、そのコードを保存する
				btn_h = btn_station_re_button
				hold = 0
			}
		} else {
			// もし過去に押されていたら、現在それがどうなっているか調べる
			if rpio.Pin(pin_re_button).Read() == rpio.Low {
				// 引き続き押されている
				hold++
				if hold > btn_press_long_width {
					hold--
					//~ time.Sleep(100*time.Millisecond)// リピート幅調整用
					oneshotlight()
					code <- (btn_h | btn_station_repeat) // リピート入力
				}
			} else {
				if hold >= btn_press_long_width {
					code <- btn_station_repeat_end // リピート入力の終わり(ボタン長押し)
				} else if hold > btn_press_width {
					oneshotlight()
					code <- btn_h // ワンショット入力
				}
				btn_h = 0
				hold = 0
			}
		}
	}
}

func afamp_enable() {
	rpio.Pin(pin_afamp).High()
}

func afamp_disable() {
	rpio.Pin(pin_afamp).Low()
}

func led_green_on() {
	rpio.Pin(pin_re_led1).Low() // 緑
}

func led_green_off() {
	rpio.Pin(pin_re_led1).High()
}

func led_red_on() {
	rpio.Pin(pin_re_led2).Low() // 赤
}

func led_red_off() {
	rpio.Pin(pin_re_led2).High()
}

func led_yellow_on() {
	led_green_on()
	led_red_on()
}

func led_yellow_off() {
	led_green_off()
	led_red_off()
}

func tune() {
	var (
		station_url string
		err         error = nil
	)

	if radio_enable && lastpos == pos {
		return
	}
	infoupdate(0, &stlist[pos].Name, false)

	args := strings.Split(stlist[pos].Url, "/")
	if args[0] == "plugin:" {
		switch args[1] {
		case "afn.py":
			station_url, err = netradio.AFN_get_url_with_api(args[2])
		case "radiko.py":
			station_url, err = netradio.Radiko_get_url(args[2])
		default:
			break
		}
		if err != nil {
			return
		}
	} else {
		station_url = stlist[pos].Url
	}

	s := fmt.Sprintf("{\"command\": [\"loadfile\",\"%s\"]}\x0a", station_url)
	mpvctl.Send(s)
	radio_enable = true
	lastpos = pos
}

func radio_stop() {
	mpvctl.Stop()
	infoupdate(0, &errmessage[SPACE16], false)
	afamp_disable() // AF amp disable
	radio_enable = false
}

func alarm_time_inc() {
	if alarm_set_pos == 0 {
		alarm_time = alarm_time.Add(1 * time.Hour)
	} else {
		alarm_time = alarm_time.Add(1 * time.Minute)
	}
}

func alarm_time_dec() {
	if alarm_set_pos == 1 {
		// minute 時間が進んでしまうのでhourも補正する
		alarm_time = alarm_time.Add(59 * time.Minute)
	}
	alarm_time = alarm_time.Add(23 * time.Hour)
}

func showclock() {
	mu.Lock()
	defer mu.Unlock()
	var (
		tm string                       // 時刻
		dt string                       // 日付
		al string = " "                 // アラームオン
		sl string = " "                 // スリープオン
		bf        = make([]byte, 0, 17) // LCD転送用
	)

	if (clock_mode & clock_mode_alarm) != 0 {
		al = "A"
	}
	if (clock_mode & clock_mode_sleep) != 0 {
		sl = "S"
	}
	bf = append(bf, al...)
	bf = append(bf, sl...)

	if statepos == state_set_alarmtime {
		// アラーム時刻編集モード時は時刻表示をアラーム時刻にする
		if colon == 1 {
			if alarm_set_pos == 0 {
				// 時を点滅表示
				tm = fmt.Sprintf("   :%02d", alarm_time.Minute())
			} else {
				// 分を点滅表示
				tm = fmt.Sprintf(" %02d:  ", alarm_time.Hour())
			}
		} else {
			tm = fmt.Sprintf(" %02d:%02d", alarm_time.Hour(),
				alarm_time.Minute())
		}
	} else {
		nowlocal := time.Now().In(jst) //Local()
		tm = fmt.Sprintf(" %02d%c%02d",
			nowlocal.Hour(), display_colon[colon], nowlocal.Minute())
		dt = fmt.Sprintf("%02d-%02d %s",
			nowlocal.Month(), nowlocal.Day(), weekday[nowlocal.Weekday()])
	}
	bf = append(bf, tm...)
	lcd.PrintWithPos(0, 1, bf)

	if radio_enable {
		display_buff_len := len(display_buff)
		if display_buff_len <= 8 {
			lcd.PrintWithPos(0, 0, display_buff)
		} else {
			lcd.PrintWithPos(0, 0, display_buff[display_buff_pos:display_buff_pos+8])
			display_buff_pos++
			if display_buff_pos >= int16(display_buff_len-8) {
				display_buff_pos = 0
			}
		}
	} else {
		lcd.PrintWithPos(0, 0, []byte(dt))
	}
}

func oneshotlight() {
	if lcd.IsLightOn() == false {
		lcd.LightOn()
	}
	light_timer.Reset(10 * time.Second)
}

func main() {
	// GPIO initialize
	for {
		err := rpio.Open()
		if err != nil {
			if os.IsNotExist(err) {
				time.Sleep(5000 * time.Millisecond)
				log.Println(err)
				continue
			}
		} else {
			break
		}
	}
	defer rpio.Close()
	for _, sn := range []rpio.Pin{pin_re_button,
		pin_re_a, pin_re_b} {
		sn.Input()
		sn.PullUp()
	}
	for _, sn := range []rpio.Pin{pin_afamp,
		pin_lcd_reset, pin_lcd_backlight,
		pin_re_led1, pin_re_led2} {
		sn.Output()
		sn.PullUp()
		sn.Low()
	}

	//~ i2c, err := i2c.New(0x3c, 1)	// aqm1602y (OLED)
	i2c, err := i2c.New(0x3e, 1) // aqm0802a
	if err != nil {
		log.Fatal(err)
	}
	defer i2c.Close()

	// OLED or LCD
	lcd = aqm0802a.New(i2c, pin_lcd_reset, pin_lcd_backlight)
	lcd.Init()
	startmes := VERSION
	infoupdate(0, &startmes, false)
	lcd.LightOn()
	light_timer = time.AfterFunc(10*time.Second, lcd.LightOff)

	// rotaryencoder
	rencoder := rotaryencoder.New(pin_re_b, pin_re_a,
		oneshotlight,
		oneshotlight)
	//~ rencoder.SetSamplingTime(4)

	jst = time.FixedZone("JST", 9*60*60)

	err = mpvctl.Init(MPV_SOCKET_PATH)
	if err != nil {
		infoupdate(0, &errmessage[ERROR_MPV_FAULT], false)
		infoupdate(1, &errmessage[ERROR_HUP], false)
		log.Fatal(err)
	}
	mpvctl.SetVoltable(&voltable)

	// シグナルハンドラ
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGINT)

	stlen := setup_station_list()
	go netradio.Radiko_setup(stlist)

	if mpvctl.Open() != nil {
		infoupdate(0, &errmessage[ERROR_MPV_CONN], false)
		infoupdate(1, &errmessage[ERROR_HUP], false)
		log.Fatal(err) // time out
	}

	mpvret := make(chan string)
	go mpvctl.Recv(mpvret, func(ms mpvctl.MpvIRC) (string, bool) {
		//~ fmt.Printf("%#v\n",ms)
		if radio_enable {
			if ms.Event == "property-change" {
				if ms.Name == "metadata/by-key/icy-title" {
					return ms.Data, true
				}
			}
		}
		return "", false
	})

	colonblink := time.NewTicker(500 * time.Millisecond)

	radio_enable = false
	pos = 0
	lastpos = pos
	volume = mpvctl.Volume_max / 2
	mpvctl.Setvol(volume)
	s := "{ \"command\": [\"observe_property_string\", 1, \"metadata/by-key/icy-title\"] }"
	mpvctl.Send(s)
	colon = 0
	clock_mode = clock_mode_normal

	//~ alarm_time = time.Unix(0, 0).UTC()
	alarm_time = time.Date(2024, time.July, 4, 4, 50, 0, 0, time.UTC)
	tuneoff_time = time.Unix(0, 0).UTC()
	btncode := make(chan ButtonCode)
	btnREcode := make(chan rotaryencoder.REvector)

	go btninput(btncode)
	go rencoder.DetectLoop(btnREcode)

	// 各ステートにおけるコールバック

	// ラジオオフ（および初期状態時の動作）
	statefunc[state_radio_off].cb_click = func() {
		// ラジオオンへ
		statefunc[state_radio_off].beforetransition()
		statepos = state_volume_controle
		statefunc[state_volume_controle].startup()
	}
	statefunc[state_radio_off].cb_re_cw = func() {
		statefunc[state_radio_off].cb_click()
	}
	statefunc[state_radio_off].cb_re_ccw = func() {
	}
	statefunc[state_radio_off].cb_press = func() {
		stmp := "shutdown"
		infoupdate(0, &stmp, false)
		afamp_disable()
		time.Sleep(700 * time.Millisecond)
		cmd := exec.Command("/sbin/poweroff", "")
		cmd.Start()
		afamp_disable() // AF amp disable
		lcd.DisplayOff()
		i2c.Close()
		lcd.LightOff()
		os.Exit(0)
	}
	statefunc[state_radio_off].startup = func() {
		led_yellow_off()
		radio_stop()
	}
	statefunc[state_radio_off].beforetransition = func() {}

	// 音量調整（ラジオオン時の動作）
	statefunc[state_volume_controle].cb_click = func() {
		// 選局へ
		statefunc[state_volume_controle].beforetransition()
		statepos = state_station_tuning
		statefunc[state_station_tuning].startup()
	}
	statefunc[state_volume_controle].cb_re_cw = func() {
		volume++
		if volume > mpvctl.Volume_max {
			volume = mpvctl.Volume_max
		}
		mpvctl.Setvol(volume)
	}
	statefunc[state_volume_controle].cb_re_ccw = func() {
		volume--
		if volume < mpvctl.Volume_min {
			volume = mpvctl.Volume_min
			if radio_enable {
				statefunc[state_volume_controle].cb_press()
			}
		}
		mpvctl.Setvol(volume)
	}
	statefunc[state_volume_controle].cb_press = func() {
		// ラジオオフへ
		statefunc[state_volume_controle].beforetransition()
		statepos = state_radio_off
		statefunc[state_radio_off].startup()
	}
	statefunc[state_volume_controle].startup = func() {
		led_green_on()
		tune()
	}
	statefunc[state_volume_controle].beforetransition = func() {
		led_green_off()
	}

	// 選局
	statefunc[state_station_tuning].cb_click = func() {
		statefunc[state_station_tuning].beforetransition()
		statepos = state_volume_controle
		statefunc[state_volume_controle].startup()
	}
	statefunc[state_station_tuning].cb_re_cw = func() {
		pos++
		if pos > stlen-1 {
			pos = 0
		}
		infoupdate(0, &stlist[pos].Name, false)
	}
	statefunc[state_station_tuning].cb_re_ccw = func() {
		pos--
		if pos < 0 {
			pos = stlen - 1
		}
		infoupdate(0, &stlist[pos].Name, false)
	}
	statefunc[state_station_tuning].cb_press = func() {
		statefunc[state_station_tuning].beforetransition()
		statepos = state_select_function
		statefunc[state_select_function].startup()
	}
	statefunc[state_station_tuning].startup = func() {
		led_red_on()
	}
	statefunc[state_station_tuning].beforetransition = func() {
		led_red_off()
	}

	// アラーム・スリープの設定
	statefunc[state_select_function].cb_click = func() {
		clock_mode++
		clock_mode &= 3
		if (clock_mode & clock_mode_sleep) != 0 {
			// スリープ時刻の設定を行う
			tuneoff_time = time.Now().In(jst).Add(30 * time.Minute) // Local().
		}
		if clock_mode == 0 {
			statefunc[state_select_function].beforetransition()
			statepos = state_set_alarmtime
			statefunc[state_set_alarmtime].startup()
		}
	}
	statefunc[state_select_function].cb_re_cw = func() {}
	statefunc[state_select_function].cb_re_ccw = func() {}
	statefunc[state_select_function].cb_press = func() {
		statefunc[state_select_function].beforetransition()
		statepos = state_volume_controle
		statefunc[state_volume_controle].startup()
	}
	statefunc[state_select_function].startup = func() {
		led_yellow_on()
	}
	statefunc[state_select_function].beforetransition = func() {
		led_yellow_off()
	}

	// アラーム時刻の設定
	statefunc[state_set_alarmtime].cb_click = func() {
		alarm_set_pos++
		if alarm_set_pos >= 2 {
			statefunc[state_set_alarmtime].beforetransition()
			statepos = state_select_function
			statefunc[state_select_function].startup()
		}
	}
	statefunc[state_set_alarmtime].cb_re_cw = func() {
		alarm_time_inc()
		showclock()
	}
	statefunc[state_set_alarmtime].cb_re_ccw = func() {
		alarm_time_dec()
		showclock()
	}
	statefunc[state_set_alarmtime].cb_press = func() {}
	statefunc[state_set_alarmtime].startup = func() {
		alarm_set_pos = 0
		led_yellow_on()
	}
	statefunc[state_set_alarmtime].beforetransition = func() {}

	statepos = state_radio_off
	statefunc[state_radio_off].startup()

	for {
		select {
		default:
			time.Sleep(10 * time.Millisecond)
			if statepos != state_set_alarmtime {
				if (clock_mode & clock_mode_alarm) != 0 {
					// アラーム時刻判定
					nowlocal := time.Now().In(jst) // Local()
					if alarm_time.Hour() == nowlocal.Hour() &&
						alarm_time.Minute() == nowlocal.Minute() {
						clock_mode ^= clock_mode_alarm
						statefunc[statepos].beforetransition()
						statepos = state_volume_controle
						statefunc[state_volume_controle].startup()
					}
				}
				if (clock_mode & clock_mode_sleep) != 0 {
					// スリープ時刻判定
					nowlocal := time.Now().In(jst) // Local()
					if tuneoff_time.Hour() == nowlocal.Hour() &&
						tuneoff_time.Minute() == nowlocal.Minute() {
						clock_mode ^= clock_mode_sleep
						statefunc[statepos].beforetransition()
						statepos = state_radio_off
						statefunc[state_radio_off].startup()
					}
				}
			}

		case title := <-mpvret:
			// mpv の応答でフィルタで処理された文字列をここで処理する
			stmp := stlist[pos].Name
			if title != "" {
				stmp = stmp + "  " + title
			}
			infoupdate(0, &stmp, true)

		case <-colonblink.C:
			colon ^= 1
			showclock()

		case r := <-btnREcode:
			switch r {
			case rotaryencoder.Forward: // ロータリーエンコーダ正進
				statefunc[statepos].cb_re_cw()

			case rotaryencoder.Backward: // ロータリーエンコーダ逆進
				statefunc[statepos].cb_re_ccw()
			}

		case r := <-btncode:
			switch r {
			case btn_station_re_button: // ロータリーエンコーダのボタン
				statefunc[statepos].cb_click()

			//~ case (btn_station_re_button|btn_station_repeat):

			case btn_station_repeat_end: // 長押し後ボタンを離した時の処理
				statefunc[statepos].cb_press()
			}

		case <-signals:
			mpvctl.Close()
			if err = mpvctl.Mpvkill(); err != nil {
				log.Println(err)
			}
			if err = os.Remove(MPV_SOCKET_PATH); err != nil {
				log.Println(err)
			}
			afamp_disable() // AF amp disable
			led_yellow_off()
			lcd.DisplayOff()
			i2c.Close()
			lcd.LightOff()
			close(signals)
			os.Exit(0)
		}
	}
}
