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
	//~ stationlist string = "/usr/local/share/mpvradio/playlists/radio.m3u"
	stationlist string = "radio.m3u"
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
	btn_station_sel2
	btn_station_mode
	btn_station_select
	btn_station_re_forward
	btn_station_re_backward
	btn_station_repeat_end
	btn_system_shutdown
	
	btn_station_repeat = 0x80
	
	btn_press_width int = 5
	btn_press_long_width int = 50
)

type EncoderMode int
const (
	encodermode_volume EncoderMode = iota 
	encodermode_tuning
	encodermode_alarmset
)
	 
const (
	clock_mode_normal uint8 = iota
	clock_mode_alarm
	clock_mode_sleep
	clock_mode_sleep_alarm
)

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

var (
	mpv	net.Conn
	lcd aqm0802a.AQM0802A
	mu sync.Mutex
	stlist []*StationInfo
	colon uint8
	pos int
	radio_enable bool
	readbuf = make([]byte, mpvIRCbuffsize)
	mpvprocess *exec.Cmd
	volume int8
	display_colon = []uint8{' ',':'}
	display_sleep = []uint8{' ',' ','S'}
	display_buff string
	display_buff_len int8
	display_buff_pos int8
	clock_mode uint8
	alarm_time time.Time
	tuneoff_time time.Time
	alarm_set_mode bool
	alarm_set_pos int
	errmessage = []string{"HUP             ",
						"mpv conn error. ",
						"mpv fault.      ",
						"                ",
						"tuning error.   ",
						"rpio can't open.",
						"socket not open."}
	
	btnscan = []rpio.Pin{6, 13, 16, 20, 21}	// sel2 mode select re_1 re_2
	ctrlport = []rpio.Pin{12, 17, 4}	// afamp lcd_reset lcd_backlight 
	encoder_mode EncoderMode
	jst *time.Location
	lcdbacklight bool
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
	display_buff_len = int8(len(*mes))
	display_buff_pos = 0
	//~ if display_buff_len >= 17 {
	if display_buff_len >= 9 {	// aqm0802a
		if line == 0 {
			display_buff = *mes + "  " + *mes
		}
		//~ lcd.PrintWithPos(0, line, []byte(*mes)[:17])
		lcd.PrintWithPos(0, line, []byte(*mes)[:9])	// aqm0802a
	} else {
		if line == 0 {
			display_buff = *mes
		}
		lcd.PrintWithPos(0, line, []byte(*mes))
	}
}

func btninput(code chan<- ButtonCode) {
	var (
		//~ re_count uint8 = 0
		//~ re_table = []int8{0,1,-1,0,-1,0,0,1,1,0,0,-1,0,-1,1,0}
		//~ re_old int8 = 0
		//~ re_now int8 = 0
	)
	hold := 0
	btn_h := btn_station_none

	for {
		time.Sleep(5*time.Millisecond)
		// ロータリーエンコーダ
		b4 := btnscan[4].Read()
		b3 := btnscan[3].Read()
		//~ b3 ^= b4	// 0,1,3,2 -> 0,1,2,3
		re_tmp := 0
		switch (b4 << 1 | b3) {
			case 0:
				if btnscan[4].EdgeDetected() {
					re_tmp += 1
				}
				if btnscan[3].EdgeDetected() {
					re_tmp += -1
				}
				btnscan[3].Detect(rpio.RiseEdge)
				btnscan[4].Detect(rpio.RiseEdge)
			case 1:
				if btnscan[3].EdgeDetected() {
					re_tmp += 1
				}
				if btnscan[4].EdgeDetected() {
					re_tmp += -1
				}
				btnscan[4].Detect(rpio.RiseEdge)
				btnscan[3].Detect(rpio.FallEdge)
			//~ case 2:
			case 3:
				if btnscan[4].EdgeDetected() {
					re_tmp += 1
				}
				if btnscan[3].EdgeDetected() {
					re_tmp += -1
				}
				btnscan[3].Detect(rpio.FallEdge)
				btnscan[4].Detect(rpio.FallEdge)
			//~ case 3:
			case 2:
				if btnscan[3].EdgeDetected() {
					re_tmp += 1
				}
				if btnscan[4].EdgeDetected() {
					re_tmp += -1
				}
				btnscan[4].Detect(rpio.FallEdge)
				btnscan[3].Detect(rpio.RiseEdge)
		}
		switch re_tmp {
			case 1:
				code <- btn_station_re_forward
			case -1:
				code <- btn_station_re_backward
			default:
		}
		
		switch btn_h {
			case 0:
				for i, sn := range(btnscan[:btn_station_select]) {
					// 押されているボタンがあれば、そのコードを保存する
					if sn.Read() == rpio.Low {
						btn_h = ButtonCode(i+1)
						hold = 0
						break
					}
				}

			// もし過去になにか押されていたら、現在それがどうなっているか調べる
			default:
				for i, sn := range(btnscan[:btn_station_select]) {
					if btn_h == ButtonCode(i+1) {
						if sn.Read() == rpio.Low {
							// 引き続き押されている
							hold++
							if hold > btn_press_long_width {
								if btn_h == btn_station_mode {
									// mode と selectの同時押しの特殊処理
									if btnscan[btn_station_select-1].Read() == rpio.Low { 
										btn_h = btn_system_shutdown
									}
								}
								// リピート入力
								// 表示が追いつかないのでリピート幅を調整すること
								hold--
								time.Sleep(100*time.Millisecond)
								code <- (btn_h | btn_station_repeat)
							}
						} else {
							if hold >= btn_press_long_width {
								// リピート入力の終わり
								code <- btn_station_repeat_end
							} else if hold > btn_press_width {
								// ワンショット入力
								code <- btn_h
							}
							btn_h = 0
							hold = 0
						}
						break
					}
				}
		}
	}
}

func afamp_enable() {
	rpio.Pin(12).High()
}

func afamp_disable() {
	rpio.Pin(12).Low()
}

func lcdlight_on() {
	rpio.Pin(4).High()
	lcdbacklight = true
}

func lcdlight_off() {
	rpio.Pin(4).Low()
	lcdbacklight = false
}

func lcdreset() {
	rpio.Pin(17).Low()
	time.Sleep(100*time.Millisecond)
	rpio.Pin(17).High()
}

func tune() {
	var (
		station_url string
		err error = nil
	)
	infoupdate(0, &stlist[pos].name)
	
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
}

func radio_stop() {
	mpv_send("{\"command\": [\"stop\"]}\x0a")
	infoupdate(0, &errmessage[SPACE16])
	afamp_disable()		// AF amp disable
	radio_enable = false
}

func alarm_time_inc() {
	if alarm_set_pos == 0 {
		alarm_time = alarm_time.Add(1*time.Hour)
	} else {
		alarm_time = alarm_time.Add(1*time.Minute)
	}
	//~ alarm_time = time.Date(2009, 1, 1, alarm_time.Hour(), alarm_time.Minute(), 0, 0, time.UTC)
}

func alarm_time_dec() {
	if alarm_set_pos == 1 {
		// minute 時間が進んでしまうのでhourも補正する
		alarm_time = alarm_time.Add(59*time.Minute)
	}
	// hour
	alarm_time = alarm_time.Add(23*time.Hour)
	//~ alarm_time = time.Date(2009, 1, 1, alarm_time.Hour(), alarm_time.Minute(), 0, 0, time.UTC)
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
	
	if alarm_set_mode {
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
	
	// １行目の表示
	// 文字列があふれる場合はスクロールする
	//~ if display_buff_len <= 16 {
	if display_buff_len <= 8 {	// aqm0802a
		lcd.PrintWithPos(0, 0, []byte(display_buff))
	} else {
		if encoder_mode == encodermode_tuning {
			// 選局操作中はスクロールしない
			lcd.PrintWithPos(0, 0, []byte(display_buff)[0:9])
		} else {
			lcd.PrintWithPos(0, 0, []byte(display_buff)[display_buff_pos:display_buff_pos+9])
			display_buff_pos++
			if display_buff_pos >= int8(display_buff_len + 2) {
				display_buff_pos = 0
			} 
		}
	}
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
	for _, sn := range(btnscan) {
		sn.Input()
		sn.PullUp()
	}
	for _, sn := range(ctrlport) {
		sn.Output()
		sn.PullUp()
		sn.Low()
	}

	// OLED or LCD
	lcdreset()
	lcdlight_on()

	//~ i2c, err := i2c.New(0x3c, 1)	// aqm1602y (OLED)
	i2c, err := i2c.New(0x3e, 1)	// aqm0802a
	if err != nil {
		log.Fatal(err)
	}
	defer i2c.Close()
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
		infoupdate(0, &errmessage[ERROR_SOCKET_NOT_OPEN])
		infoupdate(1, &errmessage[ERROR_HUP])
		log.Fatal(err)
	}
	defer radiosocket.Close()

	err = mpvprocess.Start()
	if err != nil {
		infoupdate(0, &errmessage[ERROR_MPV_FAULT])
		infoupdate(1, &errmessage[ERROR_HUP])
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
			infoupdate(0, &errmessage[ERROR_MPV_CONN])
			infoupdate(1, &errmessage[ERROR_HUP])
			log.Fatal(err)	// time out
		}
	}

	colonblink := time.NewTicker(500*time.Millisecond)
	
	radio_enable = false
	pos = 0
	volume = 60
	mpv_setvol(volume)
	colon = 0
	clock_mode = clock_mode_normal
	
	select_btn_repeat_count := 0

	mode_btn_repeat_count := 0
	alarm_set_mode = false
	alarm_set_pos = 0
	alarm_time = time.Unix(0, 0).UTC()
	tuneoff_time = time.Unix(0, 0).UTC()
	btncode := make(chan ButtonCode)
	display_buff = ""
	display_buff_pos = 0
	encoder_mode = encodermode_volume
	
	go btninput(btncode)
	go recv_title(radiosocket)
	
	for {
		select {
			default:
				time.Sleep(10*time.Millisecond)
				if alarm_set_mode == false {
					if (clock_mode & clock_mode_alarm) != 0 {
						// アラーム時刻判定
						nowlocal := time.Now().In(jst)	// Local()
						if alarm_time.Hour() == nowlocal.Hour() &&
						   alarm_time.Minute() == nowlocal.Minute() {
							clock_mode ^= clock_mode_alarm
							tune()
						}
					}
					if (clock_mode & clock_mode_sleep) != 0 {
						// スリープ時刻判定
						nowlocal := time.Now().In(jst)	// Local()
						if tuneoff_time.Hour() == nowlocal.Hour() &&
						   tuneoff_time.Minute() == nowlocal.Minute() {
							clock_mode ^= clock_mode_sleep
							radio_stop()
						}
					}
				}
				
			case <-colonblink.C:
				colon ^= 1
				showclock()
				
			case r := <-btncode:
				switch r {
					case btn_station_re_forward:	// ロータリーエンコーダ正進
						switch {
							case alarm_set_mode:
								// アラーム時刻設定
								alarm_time_inc()
								showclock()
							case encoder_mode == encodermode_tuning:
								// 選局
								pos++
								if pos > stlen -1 {
									pos = 0
								}
								infoupdate(0, &stlist[pos].name)
							default:
								// 音量調整
								volume++
								if volume > 99 {
									volume = 99
								}
								fmt.Println(volume)
								mpv_setvol(volume)
							}
						
					case btn_station_re_backward:	// ロータリーエンコーダ逆進
						switch {
							case alarm_set_mode:
								// アラーム時刻設定
								alarm_time_dec()
								showclock()
							case encoder_mode == encodermode_tuning:
								// 選局
								pos--
								if pos < 0 {
									pos = stlen - 1
								}
								infoupdate(0, &stlist[pos].name)
							default:
								// 音量調整
								volume--
								if volume < 0 {
									volume = 0
								}
								fmt.Println(volume)
								mpv_setvol(volume)
						}
					
					case (btn_station_sel2|btn_station_repeat):
						// LCD バックライトの制御
						if lcdbacklight {
							lcdlight_off()
						} else {
							lcdlight_on()
						}
						
					case btn_station_sel2:
						// ロータリーエンコーダへの割当機能の変更
						if radio_enable {
							if encoder_mode == encodermode_tuning {
								tune()
							}
						}
						encoder_mode++
						if encoder_mode > encodermode_alarmset {
							encoder_mode = encodermode_volume
						}
						
					case btn_system_shutdown|btn_station_repeat:
						stmp := "shutdown now    "
						infoupdate(0, &stmp)
						afamp_disable()
						time.Sleep(700*time.Millisecond)
						cmd := exec.Command("/sbin/poweroff", "")
						cmd.Run()
						
					case btn_station_mode:
						if alarm_set_mode {
							// アラーム設定時は変更桁の遷移を行う
							alarm_set_pos++
							if alarm_set_pos >= 2 {
								alarm_set_mode = false
							}
						} else {
							// 通常時はアラーム、スリープのオンオフを行う
							clock_mode++
							clock_mode &= 3
							if (clock_mode & clock_mode_sleep) != 0 {
								// スリープ時刻の設定を行う
								tuneoff_time = time.Now().In(jst).Add(30*time.Minute)	// Local().
							}
						}
						
					case (btn_station_mode|btn_station_repeat):
						// alarm set
						mode_btn_repeat_count++
						if mode_btn_repeat_count > 2 {
							// アラーム時刻の設定へ
							alarm_set_mode = true
							alarm_set_pos = 0
						}
						
					case (btn_station_select|btn_station_repeat):
						select_btn_repeat_count++
						fallthrough
						
					case btn_station_select:
						if radio_enable {
							radio_stop()
						} else {
							tune()
						}
						
					case btn_station_repeat_end:
						select_btn_repeat_count = 0
						mode_btn_repeat_count = 0
						
				}
		}
	}
}
