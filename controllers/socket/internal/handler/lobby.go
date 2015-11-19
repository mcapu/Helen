// Copyright (C) 2015  TF2Stadium
// Use of this source code is governed by the GPLv3
// that can be found in the COPYING file.

package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TF2Stadium/Helen/config"
	"github.com/TF2Stadium/Helen/controllers/broadcaster"
	chelpers "github.com/TF2Stadium/Helen/controllers/controllerhelpers"
	db "github.com/TF2Stadium/Helen/database"
	"github.com/TF2Stadium/Helen/helpers"
	"github.com/TF2Stadium/Helen/helpers/authority"
	"github.com/TF2Stadium/Helen/models"
	"github.com/TF2Stadium/wsevent"
)

func LobbyCreate(_ *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		bytes, _ := json.Marshal(reqerr)
		return bytes
	}

	var args struct {
		Map         *string `json:"map"`
		Type        *string `json:"type" valid:"debug,6s,highlander,4v4,ultiduo,bball"`
		League      *string `json:"league" valid:"ugc,etf2l,esea,asiafortress,ozfortress"`
		Server      *string `json:"server"`
		RconPwd     *string `json:"rconpwd"`
		WhitelistID *uint   `json:"whitelistID"`
		Mumble      *bool   `json:"mumbleRequired"`
	}

	err := chelpers.GetParams(data, &args)
	if err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	player, _ := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))

	var playermap = map[string]models.LobbyType{
		"debug":      models.LobbyTypeDebug,
		"6s":         models.LobbyTypeSixes,
		"highlander": models.LobbyTypeHighlander,
		"ultiduo":    models.LobbyTypeUltiduo,
		"bball":      models.LobbyTypeBball,
		"4v4":        models.LobbyTypeFours,
	}

	lobbyType := playermap[*args.Type]

	randBytes := make([]byte, 6)
	rand.Read(randBytes)
	serverPwd := base64.URLEncoding.EncodeToString(randBytes)

	//TODO what if playermap[lobbytype] is nil?
	info := models.ServerRecord{
		Host:           *args.Server,
		RconPassword:   *args.RconPwd,
		ServerPassword: serverPwd}
	// err = models.VerifyInfo(info)
	// if err != nil {
	// 	bytes, _ := helpers.NewTPErrorFromError(err).Encode()
	// 	return string(bytes)
	// }

	lob := models.NewLobby(*args.Map, lobbyType, *args.League, info, int(*args.WhitelistID), *args.Mumble)
	lob.CreatedBySteamID = player.SteamId
	lob.RegionCode, lob.RegionName = chelpers.GetRegion(*args.Server)
	if (lob.RegionCode == "" || lob.RegionName == "") && config.Constants.GeoIP != "" {
		return helpers.NewTPError("Couldn't find region server.", 1).Encode()
	}

	err = lob.SetupServer()
	if err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	lob.State = models.LobbyStateWaiting
	lob.Save()

	reply_str := struct {
		ID uint `json:"id"`
	}{lob.ID}

	models.FumbleLobbyCreated(lob)

	bytes, _ := chelpers.BuildSuccessJSON(reply_str).Encode()
	return bytes
}

func ServerVerify(server *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()
	}

	var args struct {
		Server  *string `json:"server"`
		Rconpwd *string `json:"rconpwd"`
	}

	if err := chelpers.GetParams(data, &args); err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	info := models.ServerRecord{
		Host:         *args.Server,
		RconPassword: *args.Rconpwd,
	}
	err := models.VerifyInfo(info)
	if err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	return chelpers.EmptySuccessJS
}

var timeoutStop = make(map[uint](chan struct{}))

func LobbyClose(server *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()
	}

	var args struct {
		Id *uint `json:"id"`
	}

	if err := chelpers.GetParams(data, &args); err != nil {
		return helpers.NewTPErrorFromError(err).Encode()

	}

	player, _ := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))

	lob, tperr := models.GetLobbyByIdServer(uint(*args.Id))
	if tperr != nil {
		return tperr.Encode()
	}

	if player.SteamId != lob.CreatedBySteamID && player.Role != helpers.RoleAdmin {
		return helpers.NewTPError("Player not authorized to close lobby.", -1).Encode()

	}

	if lob.State == models.LobbyStateEnded {
		return helpers.NewTPError("Lobby already closed.", -1).Encode()
	}

	models.FumbleLobbyEnded(lob)

	lob.Close(true)
	models.BroadcastLobbyList() // has to be done manually for now

	c, ok := timeoutStop[*args.Id]
	if !ok {
		close(c)
	}

	return chelpers.EmptySuccessJS
}

func LobbyJoin(server *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()

	}

	var args struct {
		Id    *uint   `json:"id"`
		Class *string `json:"class"`
		Team  *string `json:"team" valid:"red,blu"`
	}

	if err := chelpers.GetParams(data, &args); err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}
	//helpers.Logger.Debug("id %d class %s team %s", *args.Id, *args.Class, *args.Team)

	player, tperr := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))

	if tperr != nil {
		return tperr.Encode()
	}

	lob, tperr := models.GetLobbyById(*args.Id)
	if tperr != nil {
		return tperr.Encode()
	}

	if lob.State == models.LobbyStateEnded {
		return helpers.NewTPError("Cannot join a closed lobby.", -1).Encode()

	}

	//Check if player is in the same lobby
	var sameLobby bool
	if id, err := player.GetLobbyId(); err == nil && id == *args.Id {
		sameLobby = true
	}

	slot, tperr := models.LobbyGetPlayerSlot(lob.Type, *args.Team, *args.Class)
	if tperr != nil {
		return tperr.Encode()

	}

	tperr = lob.AddPlayer(player, slot)

	if tperr != nil {
		return tperr.Encode()
	}

	if !sameLobby {
		chelpers.AfterLobbyJoin(server, so, lob, player)
	}

	if lob.IsFull() {
		lob.State = models.LobbyStateReadyingUp
		lob.ReadyUpTimestamp = time.Now().Unix() + 30
		lob.Save()

		tick := time.After(time.Second * 30)
		id := lob.ID
		timeoutStop[id] = make(chan struct{})

		go func() {
			select {
			case <-tick:
				lobby := &models.Lobby{}
				db.DB.First(lobby, id)

				if lobby.State != models.LobbyStateInProgress {
					err := lobby.RemoveUnreadyPlayers()
					if err != nil {
						helpers.Logger.Error("RemoveUnreadyPlayers: ", err.Error())
						err = nil
					}

					err = lobby.UnreadyAllPlayers()
					if err != nil {
						helpers.Logger.Error("UnreadyAllPlayers: ", err.Error())
					}

					lobby.State = models.LobbyStateWaiting
					lobby.Save()
				}

			case <-timeoutStop[id]:
				return
			}
		}()

		room := fmt.Sprintf("%s_private",
			chelpers.GetLobbyRoom(lob.ID))
		broadcaster.SendMessageToRoom(room, "lobbyReadyUp",
			`{"timeout":30}`)
		models.BroadcastLobbyList()
	}

	models.AllowPlayer(*args.Id, player.SteamId, *args.Team+*args.Class)
	models.BroadcastLobbyToUser(lob, player.SteamId)

	return chelpers.EmptySuccessJS
}

func LobbySpectatorJoin(server *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	var noLogin bool
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		noLogin = true
		// bytes, _ := json.Marshal(reqerr)
		// return string(bytes)
	}

	var args struct {
		Id *uint `json:"id"`
	}

	if err := chelpers.GetParams(data, &args); err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	var lob *models.Lobby
	lob, tperr := models.GetLobbyById(*args.Id)

	if tperr != nil {
		return tperr.Encode()
	}

	if noLogin {
		chelpers.AfterLobbySpec(server, so, lob)
		bytes, _ := json.Marshal(models.DecorateLobbyData(lob, true))

		so.EmitJSON(helpers.NewRequest("lobbyData", string(bytes)))

		return chelpers.EmptySuccessJS
	}

	player, tperr := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))
	if tperr != nil {
		return tperr.Encode()
	}

	var specSameLobby bool

	arr, tperr := player.GetSpectatingIds()
	if len(arr) != 0 {
		for _, id := range arr {
			if id == *args.Id {
				specSameLobby = true
				continue
			}

			lobby, _ := models.GetLobbyById(id)
			lobby.RemoveSpectator(player, true)

			server.RemoveClient(so.Id(), fmt.Sprintf("%d_public", id))
		}
	}

	// If the player is already in the lobby (either joined a slot or is spectating), don't add them.
	// Just Broadcast the lobby to them, so the frontend displays it.
	if id, _ := player.GetLobbyId(); id != *args.Id && !specSameLobby {
		tperr = lob.AddSpectator(player)

		if tperr != nil {
			return tperr.Encode()
		}

		chelpers.AfterLobbySpec(server, so, lob)
	}

	models.BroadcastLobbyToUser(lob, player.SteamId)
	return chelpers.EmptySuccessJS
}

func LobbyKick(server *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()
	}

	var args struct {
		Id      *uint   `json:"id"`
		Steamid *string `json:"steamid"`
		Ban     *bool   `json:"ban" empty:"false"`
	}

	if err := chelpers.GetParams(data, &args); err != nil {
		return helpers.NewTPErrorFromError(err).Encode()
	}

	steamid := *args.Steamid
	var self bool

	selfSteamid := chelpers.GetSteamId(so.Id())
	// TODO check authorization, currently can kick anyone

	if steamid == "" || steamid == selfSteamid {
		self = true
		steamid = selfSteamid
	}

	if self && *args.Ban {
		return helpers.NewTPError("Player can't ban himself.", -1).Encode()

	}

	//player to kick
	player, tperr := models.GetPlayerBySteamId(steamid)
	if tperr != nil {
		return tperr.Encode()
	}

	playerRequesting, tperr2 := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))
	if tperr2 != nil {
		return tperr2.Encode()
	}

	lob, tperr := models.GetLobbyById(*args.Id)
	if tperr != nil {
		return tperr.Encode()
	}

	switch lob.State {
	case models.LobbyStateInProgress:
		return helpers.NewTPError("Lobby is in progress.", 1).Encode()
	case models.LobbyStateEnded:
		return helpers.NewTPError("Lobby has closed.", 1).Encode()
	}

	if !self && selfSteamid != lob.CreatedBySteamID && playerRequesting.Role != helpers.RoleAdmin {
		return helpers.NewTPError(
			"Not authorized to kick players", 1).Encode()
	}

	_, err := lob.GetPlayerSlot(player)

	var spec bool
	if err == nil {
		lob.RemovePlayer(player)
	} else if player.IsSpectatingId(lob.ID) {
		spec = true
		lob.RemoveSpectator(player, true)
	} else {
		return helpers.NewTPError("Player neither playing nor spectating", 2).Encode()
	}

	if *args.Ban {
		fmt.Println(playerRequesting.Role)
		if playerRequesting.Role == helpers.RoleAdmin {
			lob.BanPlayer(player)
		} else {
			return helpers.NewTPError(
				"Not authorized to ban players", 1).Encode()
		}
	}

	if !self {
		so, _ = broadcaster.GetSocket(player.SteamId)
	}

	if !spec {
		chelpers.AfterLobbyLeave(server, so, lob, player)
	} else {
		chelpers.AfterLobbySpecLeave(server, so, lob)
	}

	if !self {
		broadcaster.SendMessage(steamid, "sendNotification",
			fmt.Sprintf(`{"notification": "You have been removed from Lobby #%d"}`,
				*args.Id))

	}

	return chelpers.EmptySuccessJS
}

func PlayerReady(_ *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()
	}

	steamid := chelpers.GetSteamId(so.Id())
	player, tperr := models.GetPlayerBySteamId(steamid)
	if tperr != nil {
		return tperr.Encode()
	}

	lobbyid, tperr := player.GetLobbyId()
	if tperr != nil {
		return tperr.Encode()
	}

	lobby, tperr := models.GetLobbyByIdServer(lobbyid)
	if tperr != nil {
		return tperr.Encode()
	}

	if lobby.State != models.LobbyStateReadyingUp {
		return helpers.NewTPError("Lobby hasn't been filled up yet.", 4).Encode()
	}

	tperr = lobby.ReadyPlayer(player)

	if tperr != nil {
		return tperr.Encode()
	}

	if lobby.IsEveryoneReady() {
		close(timeoutStop[lobby.ID])
		lobby.State = models.LobbyStateInProgress
		lobby.Save()
		bytes, _ := json.Marshal(models.DecorateLobbyConnect(lobby))
		room := fmt.Sprintf("%s_private",
			chelpers.GetLobbyRoom(lobby.ID))
		broadcaster.SendMessageToRoom(room,
			"lobbyStart", string(bytes))
		models.BroadcastLobbyList()

		models.FumbleLobbyStarted(lobby)
	}

	return chelpers.EmptySuccessJS
}

func PlayerNotReady(_ *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	reqerr := chelpers.FilterRequest(so, authority.AuthAction(0), true)

	if reqerr != nil {
		return reqerr.Encode()
	}

	player, tperr := models.GetPlayerBySteamId(chelpers.GetSteamId(so.Id()))

	if tperr != nil {
		return tperr.Encode()
	}

	lobbyid, tperr := player.GetLobbyId()
	if tperr != nil {
		return tperr.Encode()
	}

	lobby, tperr := models.GetLobbyById(lobbyid)
	if tperr != nil {
		return tperr.Encode()
	}

	if lobby.State != models.LobbyStateReadyingUp {
		return helpers.NewTPError("Lobby hasn't been filled up yet.", 4).Encode()
	}

	tperr = lobby.UnreadyPlayer(player)
	lobby.RemovePlayer(player)

	if tperr != nil {
		return tperr.Encode()
	}

	lobby.UnreadyAllPlayers()
	c, ok := timeoutStop[lobby.ID]
	if ok {
		close(c)
	}

	return chelpers.EmptySuccessJS
}

func RequestLobbyListData(_ *wsevent.Server, so *wsevent.Client, data []byte) []byte {
	var lobbies []models.Lobby
	db.DB.Where("state = ?", models.LobbyStateWaiting).Order("id desc").Find(&lobbies)
	list, _ := json.Marshal(models.DecorateLobbyListData(lobbies))
	so.EmitJSON(helpers.NewRequest("lobbyListData", string(list)))

	return chelpers.EmptySuccessJS
}
