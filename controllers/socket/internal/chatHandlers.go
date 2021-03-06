// Copyright (C) 2015  TF2Stadium
// Use of this source code is governed by the GPLv3
// that can be found in the COPYING file.

package handler

import (
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/TF2Stadium/Helen/config"
	"github.com/TF2Stadium/Helen/controllers/broadcaster"
	chelpers "github.com/TF2Stadium/Helen/controllers/controllerhelpers"
	"github.com/TF2Stadium/Helen/helpers"
	"github.com/TF2Stadium/Helen/models"
	"github.com/bitly/go-simplejson"
	"github.com/googollee/go-socket.io"
)

var chatSendFilter = chelpers.FilterParams{
	FilterLogin: true,
	Params: map[string]chelpers.Param{
		"message": chelpers.Param{Kind: reflect.String},
		"room":    chelpers.Param{Kind: reflect.Int},
	},
}

func ChatSend(so socketio.Socket) func(string) string {
	return chelpers.FilterRequest(so, chatSendFilter,
		func(params map[string]interface{}) string {
			message := params["message"].(string)
			room := params["room"].(int)

			player, tperr := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))
			if tperr != nil {
				bytes, _ := tperr.ErrorJSON().Encode()
				return string(bytes)
			}

			helpers.Logger.Debug("received chat message: %s %s", message, player.Name)

			spec := player.IsSpectatingId(uint(room))
			//Check if player has either joined, or is spectating lobby
			lobbyId, tperr := player.GetLobbyId()
			if room > 0 {
				if tperr != nil && !spec && lobbyId != uint(room) {
					bytes, _ := chelpers.BuildFailureJSON("Player is not in the lobby.", 5).Encode()
					return string(bytes)
				}
			} else {
				// else room is the lobby list room
				room, _ = strconv.Atoi(config.Constants.GlobalChatRoom)
			}

			chatMessage := simplejson.New()
			// TODO send proper timestamps
			chatMessage.Set("timestamp", time.Now().Unix())
			chatMessage.Set("message", message)
			chatMessage.Set("room", room)

			chatMessage.Set("player", models.DecoratePlayerSummaryJson(player))
			bytes, _ := chatMessage.Encode()
			broadcaster.SendMessageToRoom(fmt.Sprintf("%s_public",
				chelpers.GetLobbyRoom(uint(room))),
				"chatReceive", string(bytes))

			resp, _ := chelpers.BuildSuccessJSON(simplejson.New()).Encode()

			chelpers.LogChat(uint(room), player.Name, message)

			chelpers.AddScrollbackMessage(uint(room), string(bytes))
			return string(resp)
		})
}
