package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"bufio"
	"log"
	"strings"
	"net"
	"time"
	"sync"
	//~ "bytes"
	//~ "encoding/json"
	"github.com/davecheney/i2c"
	"github.com/stianeikeland/go-rpio/v4"
	"local.packages/aqm0802a"
	"local.packages/netradio"
)

const (
	stationlist string = "/home/sakai/program/radio.m3u"
	MPV_SOCKET_PATH string = "/run/mpvsocket"
	MPVOPTION1     string = "--idle"
	MPVOPTION2     string = "--input-ipc-server="+MPV_SOCKET_PATH
	MPVOPTION3     string = "--no-video"
	MPVOPTION4     string = "--no-cache"
	MPVOPTION5     string = "--stream-buffer-size=256KiB"
	MPVOPTION6	   string = "--script=/home/pi/bin/title_trigger.lua"
	mpvIRCbuffsize int = 1024
	RADIO_SOCKET_PATH string = "/run/mpvradio"
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
	
	btn_press_width int = 5
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
	cb_click			func()
	cb_re_cw			func()
	cb_re_ccw			func()
	cb_press			func()
	startup				func()
	beforetransition 	func()
} 

type StationInfo struct {
	name string
	url string
}

type MpvIRCdata struct {
	Filename	*string		`json:"filename"`
	Current		bool		`json:"current"`
	Playing		bool		`json:"playing"`
}
 
type mpvIRC struct {
    Data       	*MpvIRCdata	 `json:"data"`
	Request_id  *int	 `json:"request_id"`
    Err 		string	 `json:"error"`
    Event		string	 `json:"event"`
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
	pin_re_button 	= 13
	pin_re_forward	= 19
	pin_re_backward	= 26
	
	pin_afamp		= 12
	pin_lcd_reset	= 17
	pin_lcd_backlight = 4
	pin_re_led1		= 5
	pin_re_led2		= 6
)

var (
	mpv	net.Conn
	lcd aqm0802a.AQM0802A
	mu sync.Mutex
	stlist []*StationInfo
	colon uint8
	pos int
	lastpos int
	radio_enable bool
	readbuf = make([]byte, mpvIRCbuffsize)
	mpvprocess *exec.Cmd
	volume int8
	display_colon = []uint8{' ',':'}
	display_sleep = []uint8{' ',' ','S'}
	display_buff string
	clock_mode uint8
	alarm_time time.Time
	tuneoff_time time.Time
	alarm_set_pos int

	errmessage = [][]byte{
		{0x48,0x55,0x50,0x20,0x20,0x20,0x20,0x20},	// HUP
		{0x6d,0x70,0x76,0xb4,0xd7,0xb0,0x20,0x20},	// mpvｴﾗｰ
		{0x6d,0x70,0x76,0xcc,0xab,0xd9,0xc4,0x20},	// mpvﾌｫﾙﾄ 
		{0x20,0x20,0x20,0x20,0x20,0x20,0x20,0x20},	// SPACE16
		{0x74,0x75,0x6e,0x65,0xb4,0xd7,0xb0,0x20},	// tuneｴﾗｰ
		{0x72,0x70,0x69,0x6f,0xb4,0xd7,0xb0,0x20},	// rpioｴﾗｰ
		{0xbf,0xb9,0xaf,0xc4,0xb4,0xd7,0xb0,0x20}}	// ｿｹｯﾄｴﾗｰ 
 
	jst *time.Location
	
	statefunc [statelength]stateEventhandlers
	statepos int
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
				stmp := new(StationInfo)
				stmp.url = s
				if len(name) < 8 {	// 表示器の桁数で調整すること
					stmp.name = string(name+"       ")[:8]	// aqm0802a
				} else {
					stmp.name = name
				}
				stlist = append(stlist, stmp)
			}
		}
	}
	return len(stlist)
}

func mpv_send(s string) {
	mpv.Write([]byte(s))
	for {
		n, err := mpv.Read(readbuf)
		if err != nil {
			log.Println(err)
			break
		}
		if n < mpvIRCbuffsize {
			break
		}
	}
}

var (
	volconv = []int8{	0,1,2,3,4,4,5,6,6,7,7,8,8,9,9,10,10,11,11,
						11,12,12,13,13,13,14,14,14,15,15,16,16,16,17,
						17,17,18,18,18,19,19,20,20,20,21,21,22,22,23,
						23,24,24,25,25,26,26,27,27,28,28,29,30,30,31,
						32,32,33,34,35,35,36,37,38,39,40,41,42,43,45,
						46,47,49,50,52,53,55,57,59,61,63,66,68,71,74,
						78,81,85,90,95,100}
)

func mpv_setvol(vol int8) {
	if vol < 1 {
		vol = 0
	} else if vol >= 100 {
		vol = 99
	} 
	s := fmt.Sprintf("{\"command\": [\"set_property\",\"volume\",%d]}\x0a",volconv[vol])
	mpv_send(s)
}

func infoupdate(line uint8, mes *string) {
	mu.Lock()
	defer mu.Unlock()
	
	if line == 0 {
		display_buff = *mes
	}
	lcd.PrintWithPos(0, line, []byte((*mes))[:8])
}

func btninput(code chan<- ButtonCode) {
	hold := 0
	btn_h := btn_station_none

	for {
		//~ time.Sleep(5*time.Millisecond)
		time.Sleep(10*time.Millisecond)
		// ロータリーエンコーダ
		b4 := rpio.Pin(pin_re_forward).Read()
		b3 := rpio.Pin(pin_re_backward).Read() 
		//~ b3 ^= b4	// 0,1,3,2 -> 0,1,2,3
		re_tmp := 0
		switch (b4 << 1 | b3) {
			case 0:
				if rpio.Pin(pin_re_forward).EdgeDetected() {
					re_tmp += 1
				}
				if rpio.Pin(pin_re_backward).EdgeDetected() {
					re_tmp += -1
				}
				rpio.Pin(pin_re_backward).Detect(rpio.RiseEdge)
				rpio.Pin(pin_re_forward).Detect(rpio.RiseEdge)
			case 1:
				if rpio.Pin(pin_re_backward).EdgeDetected() {
					re_tmp += 1
				}
				if rpio.Pin(pin_re_forward).EdgeDetected() {
					re_tmp += -1
				}
				rpio.Pin(pin_re_forward).Detect(rpio.RiseEdge)
				rpio.Pin(pin_re_backward).Detect(rpio.FallEdge)
			//~ case 2:
			case 3:
				if rpio.Pin(pin_re_forward).EdgeDetected() {
					re_tmp += 1
				}
				if rpio.Pin(pin_re_backward).EdgeDetected() {
					re_tmp += -1
				}
				rpio.Pin(pin_re_backward).Detect(rpio.FallEdge)
				rpio.Pin(pin_re_forward).Detect(rpio.FallEdge)
			//~ case 3:
			case 2:
				if rpio.Pin(pin_re_backward).EdgeDetected() {
					re_tmp += 1
				}
				if rpio.Pin(pin_re_forward).EdgeDetected() {
					re_tmp += -1
				}
				rpio.Pin(pin_re_forward).Detect(rpio.FallEdge)
				rpio.Pin(pin_re_backward).Detect(rpio.RiseEdge)
		}
		switch re_tmp {
			case 1:
				code <- btn_station_re_forward
			case -1:
				code <- btn_station_re_backward
			default:
		}
		
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
					code <- (btn_h | btn_station_repeat) // リピート入力
				}
			} else {
				if hold >= btn_press_long_width {
					code <- btn_station_repeat_end  // リピート入力の終わり(ボタン長押し)
				} else if hold > btn_press_width {
					code <- btn_h 					// ワンショット入力
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

func lcdlight_on() {
	rpio.Pin(pin_lcd_backlight).High()
}

func lcdlight_off() {
	rpio.Pin(pin_lcd_backlight).Low()
}

func lcdreset() {
	rpio.Pin(pin_lcd_reset).Low()
	time.Sleep(100*time.Microsecond)
	rpio.Pin(pin_lcd_reset).High()
}

func btn_led1_on() {
	rpio.Pin(pin_re_led1).Low()
}

func btn_led1_off() {
	rpio.Pin(pin_re_led1).High()
}

func btn_led2_on() {
	rpio.Pin(pin_re_led2).Low()
}

func btn_led2_off() {
	rpio.Pin(pin_re_led2).High()
}

func tune() {
	var (
		station_url string
		err error = nil
	)
	infoupdate(0, &stlist[pos].name)
	if radio_enable && lastpos == pos {
		return
	}
	
	args := strings.Split(stlist[pos].url, "/")
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
		station_url = stlist[pos].url
	}
	
	s := fmt.Sprintf("{\"command\": [\"loadfile\",\"%s\"]}\x0a", station_url)
	mpv_send(s)
	radio_enable = true	
	lastpos = pos
}

func radio_stop() {
	mpv_send("{\"command\": [\"stop\"]}\x0a")
	stmp := string(errmessage[SPACE16])
	infoupdate(0, &stmp)
	afamp_disable()		// AF amp disable
	radio_enable = false
}

func alarm_time_inc() {
	if alarm_set_pos == 0 {
		alarm_time = alarm_time.Add(1*time.Hour)
	} else {
		alarm_time = alarm_time.Add(1*time.Minute)
	}
}

func alarm_time_dec() {
	if alarm_set_pos == 1 {
		// minute 時間が進んでしまうのでhourも補正する
		alarm_time = alarm_time.Add(59*time.Minute)
	}
	alarm_time = alarm_time.Add(23*time.Hour)
}

func showclock() {
	mu.Lock()
	defer mu.Unlock()
	var (
		tm string		// 時刻
		al string = " "	// アラームオン
		sl string = " "	// スリープオン
		bf = make([]byte, 0, 17)	// LCD転送用
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
		nowlocal := time.Now().In(jst)	//Local()
		tm = fmt.Sprintf(" %02d%c%02d",
				nowlocal.Hour(), display_colon[colon], 
				nowlocal.Minute())
	}
	bf = append(bf, tm...)
	
	// aqm0802a
	//~ s := fmt.Sprintf("%c%c %s", al, sl, tm)
	//~ lcd.PrintWithPos(0, 1, []byte(s))
	lcd.PrintWithPos(0, 1, bf)

	lcd.PrintWithPos(0, 0, []byte(display_buff))
}


func recv_title(socket net.Listener) {
	var stmp string
	buf := make([]byte, 1024)
	for {
		n := func() int {
			conn, err := socket.Accept()
			if err != nil {
				return 0
			}
			defer conn.Close()
			n := 0
			for {
				n, err = conn.Read(buf)
				if err != nil {
					return 0
				}
				if n < 1024 {
					break
				}
			}
			conn.Write([]byte("OK\n"))
			return n
		}()
		if radio_enable == true {
			stmp = stlist[pos].name + "  " + string(buf[:n])
		} else {
			stmp = string(buf[:n]) + "  "
		}
		infoupdate(0, &stmp)
	}
}

func main() {
	// GPIO initialize
	for {
		err := rpio.Open()
		if err != nil {
			if os.IsNotExist(err) {
				time.Sleep(5000*time.Millisecond)
				log.Println(err)
				continue
			}
		} else {
			break
		}
	}
	defer rpio.Close()
	for _, sn := range([]rpio.Pin{ pin_re_button,
									pin_re_forward, pin_re_backward}) {
		sn.Input()
		sn.PullUp()
	}
	for _, sn := range([]rpio.Pin{ pin_afamp, 
									pin_lcd_reset, pin_lcd_backlight, 
									pin_re_led1, pin_re_led2}) {
		sn.Output()
		sn.PullUp()
		sn.Low()
	}

	//~ i2c, err := i2c.New(0x3c, 1)	// aqm1602y (OLED)
	i2c, err := i2c.New(0x3e, 1)	// aqm0802a
	if err != nil {
		log.Fatal(err)
	}
	defer i2c.Close()

	// OLED or LCD
	lcdreset()

	//~ lcd = aqm1602y.New(i2c)	// aqm1602y
	lcd = aqm0802a.New(i2c)
	lcd.Configure()
	lcd.PrintWithPos(0, 0, []byte("radio v2.0"))

	jst = time.FixedZone("JST", 9*60*60)
	mpvprocess = exec.Command("/usr/bin/mpv", 	MPVOPTION1, MPVOPTION2, 
												MPVOPTION3, MPVOPTION4, 
												MPVOPTION5) //, MPVOPTION6)
	
	radiosocket, err := net.Listen("unix", RADIO_SOCKET_PATH)
	if err != nil {
		lcd.PrintWithPos(0, 0, errmessage[ERROR_SOCKET_NOT_OPEN])
		lcd.PrintWithPos(0, 1, errmessage[ERROR_HUP])
		log.Fatal(err)
	}
	defer radiosocket.Close()

	err = mpvprocess.Start()
	if err != nil {
		lcd.PrintWithPos(0, 0, errmessage[ERROR_MPV_FAULT])
		lcd.PrintWithPos(0, 1, errmessage[ERROR_HUP])
		log.Fatal(err)
	}
	
	// シグナルハンドラ
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGINT) // syscall.SIGUSR1
		
		for {
			switch <-signals {
				//~ case syscall.SIGUSR1:
					//~ stmp := stlist[pos].name + "  " + mpv_get_title ()
					//~ infoupdate(0, &stmp)
				case syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGINT:
					// shutdown this program
					if err = mpvprocess.Process.Kill();err != nil {
						log.Println(err)
					}
					if err = os.Remove(MPV_SOCKET_PATH);err != nil {
						log.Println(err)
					}
					if err = os.Remove(RADIO_SOCKET_PATH);err != nil {
						log.Println(err)
					}
					afamp_disable()		// AF amp disable
					btn_led1_off()
					btn_led2_off()
					lcd.DisplayOff()
					i2c.Close()
					lcdlight_off()
					close(signals)
					os.Exit(0)
			}
		}
	}()
	
	stlen := setup_station_list()

	for i := 0; ;i++ {
		mpv, err = net.Dial("unix", MPV_SOCKET_PATH);
		if err == nil {
			defer mpv.Close()
			break
		}
		time.Sleep(200*time.Millisecond)
		if i > 60 {
			lcd.PrintWithPos(0, 0, errmessage[ERROR_MPV_CONN])
			lcd.PrintWithPos(0, 1, errmessage[ERROR_HUP])
			log.Fatal(err)	// time out
		}
	}

	colonblink := time.NewTicker(500*time.Millisecond)
	
	radio_enable = false
	pos = 0
	lastpos = pos
	volume = 60
	mpv_setvol(volume)
	colon = 0
	clock_mode = clock_mode_normal
	
	alarm_time = time.Unix(0, 0).UTC()
	tuneoff_time = time.Unix(0, 0).UTC()
	btncode := make(chan ButtonCode)
	display_buff = ""
	finetune := 0
	
	go btninput(btncode)
	go recv_title(radiosocket)

	// 各ステートにおけるコールバック
	
	// ラジオオフ（初期状態）
	statefunc[state_radio_off].cb_click = func() {
			statefunc[state_radio_off].beforetransition()
			statepos = state_volume_controle
			statefunc[state_volume_controle].startup()
	}
	statefunc[state_radio_off].cb_re_cw = func() {
			lcdlight_on() 
	}
	statefunc[state_radio_off].cb_re_ccw = func() {
			lcdlight_off() 
	}
	statefunc[state_radio_off].cb_press = func() {
			stmp := "shutdown"
			infoupdate(0, &stmp)
			afamp_disable()
			time.Sleep(700*time.Millisecond)
			cmd := exec.Command("/sbin/poweroff", "")
			cmd.Start()
			afamp_disable()		// AF amp disable
			lcd.DisplayOff()
			i2c.Close()
			lcdlight_off()
			os.Exit(0)
	}
	statefunc[state_radio_off].startup = func() {
			btn_led1_off()
			btn_led2_off()
			lcdlight_off()
			radio_stop()
	}
	statefunc[state_radio_off].beforetransition = func() {}

	// 音量調整（ラジオオン）
	statefunc[state_volume_controle].cb_click = func() {
			statefunc[state_volume_controle].beforetransition()
			statepos = state_station_tuning
			statefunc[state_station_tuning].startup()
	}
	statefunc[state_volume_controle].cb_re_cw = func() {
			volume++
			if volume > 99 {
				volume = 99
			}
			mpv_setvol(volume)
	}
	statefunc[state_volume_controle].cb_re_ccw = func() {
			volume--
			if volume < 0 {
				volume = 0
			}
			mpv_setvol(volume)
	}
	statefunc[state_volume_controle].cb_press = func() {
			statefunc[state_volume_controle].beforetransition()
			statepos = state_radio_off
			statefunc[state_radio_off].startup()
	}
	statefunc[state_volume_controle].startup = func() {
			btn_led1_on()
			tune()
	}
	statefunc[state_volume_controle].beforetransition = func() {
			btn_led1_off()
	}

	// 選局
	statefunc[state_station_tuning].cb_click = func() {
			statefunc[state_station_tuning].beforetransition()
			statepos = state_volume_controle
			statefunc[state_volume_controle].startup()
	}
	statefunc[state_station_tuning].cb_re_cw = func() {
			if finetune == 0 {
				pos++
				if pos > stlen -1 {
					pos = 0
				}
				infoupdate(0, &stlist[pos].name)
				finetune = 1	// 一度選局したらその後の入力をしばらく無視する
			} else {
				finetune--
			}
	}
	statefunc[state_station_tuning].cb_re_ccw = func() {
			if finetune == 0 {
				pos--
				if pos < 0 {
					pos = stlen - 1
				}
				infoupdate(0, &stlist[pos].name)
				finetune = 1	// 一度選局したらその後の入力をしばらく無視する
			} else {
				finetune--
			}
	}
	statefunc[state_station_tuning].cb_press = func() {
			statefunc[state_station_tuning].beforetransition()
			statepos = state_select_function
			statefunc[state_select_function].startup()
	}
	statefunc[state_station_tuning].startup = func() {
			btn_led2_on()
	}
	statefunc[state_station_tuning].beforetransition = func() {
			btn_led2_off()
	}

	// アラーム・スリープの設定
	statefunc[state_select_function].cb_click = func() {
			clock_mode++
			clock_mode &= 3
			if (clock_mode & clock_mode_sleep) != 0 {
				// スリープ時刻の設定を行う
				tuneoff_time = time.Now().In(jst).Add(30*time.Minute)	// Local().
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
			btn_led1_on()
			btn_led2_on()
	}
	statefunc[state_select_function].beforetransition = func() {
			btn_led1_off()
			btn_led2_off()
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
			if finetune == 0 {
				alarm_time_inc()
				showclock()
				finetune = 3
			} else {
				finetune--
			}
	}
	statefunc[state_set_alarmtime].cb_re_ccw = func() {
			if finetune == 0 {
				alarm_time_dec()
				showclock()
				finetune = 3
			} else {
				finetune--
			}
	}
	statefunc[state_set_alarmtime].cb_press = func() {}
	statefunc[state_set_alarmtime].startup = func() {
			alarm_set_pos = 0
			btn_led1_on()
			btn_led2_on()
	}
	statefunc[state_set_alarmtime].beforetransition = func() {}

	statepos = state_radio_off
	statefunc[state_radio_off].startup()
	
	for {
		select {
			default:
				time.Sleep(10*time.Millisecond)
				if statepos != state_set_alarmtime {
					if (clock_mode & clock_mode_alarm) != 0 {
						// アラーム時刻判定
						nowlocal := time.Now().In(jst)	// Local()
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
						nowlocal := time.Now().In(jst)	// Local()
						if tuneoff_time.Hour() == nowlocal.Hour() &&
						   tuneoff_time.Minute() == nowlocal.Minute() {
							clock_mode ^= clock_mode_sleep
							statefunc[statepos].beforetransition()
							statepos = state_radio_off
							statefunc[state_radio_off].startup()
						}
					}
				}
				
			case <-colonblink.C:
				colon ^= 1
				showclock()
				
			case r := <-btncode:
				switch r {
					case btn_station_re_forward:	// ロータリーエンコーダ正進
						statefunc[statepos].cb_re_cw()
						
					case btn_station_re_backward:	// ロータリーエンコーダ逆進
						statefunc[statepos].cb_re_ccw()

					case btn_station_re_button:		// ロータリーエンコーダのボタン
						statefunc[statepos].cb_click()
						
					case (btn_station_re_button|btn_station_repeat):

					case btn_station_repeat_end:	// 長押し後ボタンを離した時の処理
						statefunc[statepos].cb_press()
				}
		}
	}
}
