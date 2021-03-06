// Copyright (C) 2015  TF2Stadium
// Use of this source code is governed by the GPLv3
// that can be found in the COPYING file.

package routes

import (
	"net/http"

	"github.com/TF2Stadium/Helen/config"
	"github.com/TF2Stadium/Helen/controllers"
	"github.com/TF2Stadium/Helen/controllers/socket"
	"github.com/googollee/go-socket.io"
)

func SetupHTTPRoutes() {
	http.HandleFunc("/", controllers.MainHandler)
	http.HandleFunc("/openidcallback", controllers.LoginCallbackHandler)
	http.HandleFunc("/startLogin", controllers.LoginHandler)
	http.HandleFunc("/logout", controllers.LogoutHandler)
	if config.Constants.MockupAuth {
		http.HandleFunc("/startMockLogin/", controllers.MockLoginHandler)
	}
}

func SetupSocketRoutes(server *socketio.Server) {
	server.On("connection", socket.SocketInit)
}
