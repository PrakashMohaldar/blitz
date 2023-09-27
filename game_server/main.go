package main

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"

	"github.com/PrakashMohaldar/gameserver/types"
	"github.com/anthdm/hollywood/actor"
	"github.com/gorilla/websocket"
)



type PlayerSession struct {
	sessionID int
	clientID int
	username string
	inLobby bool
	conn *websocket.Conn
	ctx *actor.Context
	serverPID *actor.PID//server process id
}

// returning a producer
// creating session from websocket connection
func newPlayerSession(serverPID *actor.PID, sid int,conn *websocket.Conn) actor.Producer{
	return func() actor.Receiver {
		return &PlayerSession{
			conn: conn,
			sessionID: sid,
			serverPID: serverPID,//local/server
		}
	}
} 

// recieve is used to process incoming messages
// actor context provide info about actor' state and message is has received
func (s *PlayerSession) Receive(ctx *actor.Context){
	switch msg := ctx.Message().(type){
	case actor.Started:
		s.ctx = ctx
		go s.readLoop()// blocking so making it non blocking
	case *types.PlayerState:
		fmt.Println("I just received the state of another player", msg.SessionID)
		s.sendStateToClient(msg)
	default:
		fmt.Println("recv", msg)
	}
}

func (s * PlayerSession) sendStateToClient(state *types.PlayerState){
	b, err := json.Marshal(state)
	
	if err != nil{
		panic(err)
	}
	msg  := types.WSMessage{
		Type : "state",
		Data: b,
	}
	if err := s.conn.WriteJSON(msg); err != nil {
		panic(err)
	}
}

func (s *PlayerSession) readLoop(){
	var msg types.WSMessage
	for{
		if err := s.conn.ReadJSON(&msg); err != nil {
			fmt.Println("read error", err)
			return
		}
		go s.handleMessage(msg)
	}
}


func (s *PlayerSession) handleMessage(msg types.WSMessage){
	switch msg.Type {
	case "Login":
		var loginMsg types.Login
		if err := json.Unmarshal(msg.Data,&loginMsg); err != nil {
			panic( err )
		}
		s.clientID = loginMsg.ClientID
		s.username = loginMsg.Username
	case "playerState":
		// recieving the positions of the client
			var ps types.PlayerState
			if err := json.Unmarshal(msg.Data, &ps); err != nil {
				panic(err)
			}
			ps.SessionID = s.sessionID 

			if s.ctx != nil{
				s.ctx.Send(s.serverPID, &ps)
			}

			// fmt.Println(ps)

	}
}

type GameServer struct {
	ctx *actor.Context
	sessions map[int]*actor.PID
}


// Producer is any function that can return a Receiver
// Receiver is an interface that can receive and process messages.
// returns a pointer 
func newGameServer() actor.Receiver {
	return &GameServer{
		sessions: make(map[int]*actor.PID),
	}
}
// actor.Context gets the message sent from connected client
func (s *GameServer) Receive(ctx *actor.Context) {
	// getting context 
	switch msg := ctx.Message().(type){
	// when other player try to connect it broadcast there state to all other player
	case *types.PlayerState:
		s.bcast(ctx.Sender(), msg)
		// starting the actor, its done by hollywood spawn function 
	case actor.Started:
		s.startHTTP()
		s.ctx = ctx
		_ = msg
	default:
		fmt.Println("recv", msg)
	}


}

func (s *GameServer) bcast(from *actor.PID ,state *types.PlayerState){
	// fmt.Println("bcasting", s.sessions)
	for _, pid := range s.sessions {     
		if !pid.Equals(from){
			// fmt.Println("sending state to player", pid)
			s.ctx.Send(pid,state)//sending mesg to other childs except the current child actor who sends the msg
		}
	}
	// boradcasting state of one player to all other player
	// fmt.Printf("%+v\n", state)
}

func (s *GameServer) startHTTP(){
	fmt.Println("starting HTTP server on port: 4000 ")
	// establising the seperated thread server for each player
	go func(){
		http.HandleFunc("/ws", s.handleWS)
		http.ListenAndServe(":4000", nil)
	}()
}


// handles the upgrades of the websocket when client sends connection request
func (s *GameServer) handleWS(w http.ResponseWriter, r * http.Request){
	var upgrader = websocket.Upgrader{
		ReadBufferSize: 1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w,r, nil)

	if err != nil{
		fmt.Println("ws upgrade error:", err)
	}

	fmt.Println("new client trying to connect")
	// fmt.Print(conn)

	// creating sesssion ID
	sid := rand.Intn(math.MaxInt)
	// fmt.Println(">>>>>>>>> PID from s.ctx.PID =", s.ctx.PID())
	pid := s.ctx.SpawnChild(newPlayerSession(s.ctx.PID(),sid,conn), fmt.Sprintf("session_%d", sid))
	// fmt.Println(">>>>>>>> PID from spawnchild =", pid)

	s.sessions[sid] = pid
	fmt.Printf("client with sid %d and pid %s just connected\n", sid, pid)
}

func main(){
	e := actor.NewEngine();
	// newGamerServer is a producer
	e.Spawn(newGameServer, "server")
	select{}
}


