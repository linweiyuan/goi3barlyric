package main

import (
	"encoding/json"
	"fmt"
	"github.com/hajimehoshi/oto"
	"github.com/tosone/minimp3"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SongJson struct {
	CurrentPlaying []SongData `json:"current-playing"`
}

type SongData struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Source string `json:"source"`
}

type I3Bar struct {
	FullText string `json:"full_text"`
	Color    string `json:"color"`
}

var (
	playSongCh = make(chan bool)
	context    *oto.Context
	player     *oto.Player
)

func init() {
	rand.Seed(time.Now().UnixNano())
	fmt.Println(`{"version": 1}`)
	fmt.Println(`[[]`)
}

func main() {
	resp, err := http.Get(`https://gist.githubusercontent.com/linweiyuan/9510499950f9be5576fdb77a20029c00/raw/3ba53f50eda07023b66cb6a705a8f2a1b33fa27f/listen1_backup.json`)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var songJson SongJson
	json.Unmarshal(bytes, &songJson)

	var netEaseSongs []SongData
	for _, song := range songJson.CurrentPlaying {
		if song.Source == "netease" {
			song.ID = song.ID[strings.Index(song.ID, "_")+1:]
			netEaseSongs = append(netEaseSongs, song)
		}
	}

	go func() {
		playSongCh <- false
	}()

	for {
		<-playSongCh
		randomSong := netEaseSongs[rand.Intn(len(netEaseSongs))]
		go playSong(randomSong)
	}
}

func playSong(song SongData) {
	resp, err := http.Get(fmt.Sprintf(`https://n.xlz122.cn/api/song/url?id=%s`, song.ID))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var songResponse struct {
		Data []struct {
			URL           string `json:"url"`
			FreeTrialInfo *struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"freeTrialInfo"`
		} `json:"data"`
		Code int `json:"code"`
	}
	json.Unmarshal(bytes, &songResponse)
	if songResponse.Code != 200 || songResponse.Data[0].FreeTrialInfo != nil {
		playSongCh <- false
		return
	}
	songUrl := songResponse.Data[0].URL
	response, err := http.Get(songUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	decoder, _ := minimp3.NewDecoder(response.Body)
	<-decoder.Started()

	if context == nil {
		context, _ = oto.NewContext(decoder.SampleRate, decoder.Channels, 2, 4096)
		player = context.NewPlayer()
	}

	go showLyric(song)

	var waitForPlayOver = new(sync.WaitGroup)
	waitForPlayOver.Add(1)

	var data = make([]byte, 512)
	for {
		n, err := decoder.Read(data)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		player.Write(data[:n])
	}
	playSongCh <- false
}

func showLyric(song SongData) {
	resp, err := http.Get(fmt.Sprintf(`https://n.xlz122.cn/api/lyric?id=%s`, song.ID))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	var lyricResponse struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	json.Unmarshal(bytes, &lyricResponse)
	lyric := lyricResponse.Lrc.Lyric

	var tempMilliSeconds = 0
	for _, lyricLine := range strings.Split(lyric, "\n") {
		timeLyricStrings := strings.Split(lyricLine, "]")
		if len(timeLyricStrings) != 2 {
			continue
		}
		timeStrings := timeLyricStrings[0][1:]
		minuteStrings := strings.Split(timeStrings, ":")[0]
		secondsStrings := strings.Split(strings.Split(timeStrings, ":")[1], ".")[0]
		milliSecondsStrings := strings.Split(strings.Split(timeStrings, ":")[1], ".")[1]

		minutes, _ := strconv.Atoi(minuteStrings)
		seconds, _ := strconv.Atoi(secondsStrings)
		milliSeconds, _ := strconv.Atoi(milliSecondsStrings)

		totalMilliSeconds := minutes*60*1000 + seconds*1000 + milliSeconds
		time.Sleep(time.Millisecond * time.Duration(totalMilliSeconds-tempMilliSeconds))
		tempMilliSeconds = totalMilliSeconds

		lrc := strings.TrimSpace(timeLyricStrings[1])
		if lrc == "" {
			lrc = "music..."
		}
		lrc += fmt.Sprintf(` (%s - %s)`, song.Title, song.Artist)
		fmt.Println(",[")
		bytes, _ := json.Marshal(I3Bar{
			FullText: lrc,
			Color:    "#00FF00",
		})
		fmt.Println(string(bytes))
		fmt.Println("]")
	}
}
