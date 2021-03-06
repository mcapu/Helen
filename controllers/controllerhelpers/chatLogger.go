// Copyright (C) 2015  TF2Stadium
// Use of this source code is governed by the GPLv3
// that can be found in the COPYING file.

package controllerhelpers

import (
	"fmt"
	"github.com/TF2Stadium/Helen/config"
	"github.com/TF2Stadium/Helen/helpers"
	"github.com/TF2Stadium/Helen/models"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var mapLock = &sync.Mutex{}
var roomLogChannel = make(map[uint](chan string))

var globalLog *os.File
var globalLogLock = &sync.Mutex{}

func StartGlobalLogger() {
	if !config.Constants.ChatLogsEnabled {
		return
	}
	go globalLogFileUpdater()
}

func globalLogFileUpdater() {
	init := true
	var now time.Time
	ticker := time.NewTicker(time.Duration(24 - time.Now().Hour()))
	for {
		if !init {
			now = <-ticker.C
			ticker.Stop()
			ticker = time.NewTicker(time.Hour * 24)
		} else {
			now = time.Now()
			roomLogChannel[0] = make(chan string, 18)
		}
		globalLogLock.Lock()
		globalLog.Close()
		year, month, day := now.Date()
		filename := fmt.Sprintf("room#0-%d-%s-%d", day,
			month.String(), year)

		globalLog, err := os.OpenFile(filename,
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			helpers.Logger.Critical("%s", err.Error())
			continue
		}
		if !init {
			StopLogger(0)
		}
		globalLogLock.Unlock()
		go logListener(roomLogChannel[0], globalLog, 0)
		init = false
	}
}

func logListener(channel <-chan string, file *os.File, room uint) {
	for {
		message, open := <-channel
		if !open {
			helpers.Logger.Debug("Stopping listener for #%d", room)
			file.Close()
			return
		}
		if room == 0 {
			globalLogLock.Lock()
		}
		file.WriteString(message)
		if room == 0 {
			globalLogLock.Unlock()
		}
	}
}

func LogChat(room uint, player string, message string) {
	if !config.Constants.ChatLogsEnabled {
		return
	}
	mapLock.Lock()
	channel, exists := roomLogChannel[room]
	if !exists {
		roomLogChannel[room] = make(chan string, 18)
		channel = roomLogChannel[room]
	}

	mapLock.Unlock()
	now := time.Now()

	if !exists && room != 0 {
		year, month, day := now.Date()
		path, err := filepath.Abs(config.Constants.ChatLogsDir)
		if err != nil {
			helpers.Logger.Critical("%s", err.Error())
			return
		}

		directory := fmt.Sprintf("%s/%d-%s-%d",
			path, day, month.String(), year)
		filename := fmt.Sprintf("%s/room#%d", directory, room)
		helpers.Logger.Debug("%s %s", directory, filename)
		err = os.Mkdir(directory, 0777)
		if err != nil {
			helpers.Logger.Critical("%s", err.Error())
			return
		}

		file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY,
			0600)
		go logListener(channel, file, room)
	}

	entry := fmt.Sprintf("[%d:%d] <%s>: %s\n", now.Hour(), now.Minute(),
		player, message)
	channel <- entry
}

func WriteLobbyInfo(file *os.File, lobby *models.Lobby) {
	if !config.Constants.ChatLogsEnabled {
		return
	}
	file.Seek(0, os.SEEK_SET)
	//TODO: write lobby info to file
}

func StopLogger(room uint) {
	if !config.Constants.ChatLogsEnabled {
		return
	}
	mapLock.Lock()
	close(roomLogChannel[room])
	delete(roomLogChannel, room)
	mapLock.Unlock()
}
