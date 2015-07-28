package models

import (
	"testing"

	"github.com/TF2Stadium/Server/config"
	"github.com/TF2Stadium/Server/helpers"
	"github.com/stretchr/testify/assert"
)

// change this if you wanna test the server
// make sure you have the server running at the moment
var shouldTest bool = false
var svr *Server

func init() {
	helpers.InitLogger()
}

func TestServerSetup(t *testing.T) {
	if shouldTest {
		config.SetupConstants()
		InitServerConfigs()

		commId := "76561198067132047" // your commId, so it wont be kicking you out everytime

		info := ServerRecord{
			Host:         "192.168.1.94:27015",
			RconPassword: "rconPassword",
		}

		svr = NewServer()
		svr.Map = "cp_process_final"
		svr.Type = LobbyTypeHighlander
		svr.League = LeagueUgc
		svr.LobbyPassword = "12345"
		svr.Info = info

		svr.AllowPlayer(commId)

		setupErr := svr.Setup()
		assert.Nil(t, setupErr, "Setup error")

		playerIsAllowed := svr.IsPlayerAllowed(commId)
		assert.True(t, playerIsAllowed, "Player isn't allowed, he should")

		svr.DisallowPlayer(commId)

		playerIsntAllowed := svr.IsPlayerAllowed(commId)
		assert.False(t, playerIsntAllowed, "Player is allowed, he shouldn't")

		svr.End()
	}
}