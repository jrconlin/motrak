package motrak

import (
	"code.google.com/p/go.net/websocket"
	"github.com/jrconlin/motrak/util"

	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

var (
	Clients *ClientBox
)

func init() {
    Clients = &ClientBox{
        clients: make(map[string]WWS),
    }
}


type ClientBox struct {
	sync.RWMutex
    clients map[string]WWS
}

func (r *ClientBox) Add(key string, ws WWS) {
	defer r.Unlock()
	r.Lock()
	r.clients[key] = ws
}

func (r *ClientBox) Get(key string) WWS {
	defer r.Unlock()
	r.Lock()
	return r.clients[key]
}

func (r *ClientBox) Del(key string) {
	defer r.Unlock()
	r.Lock()
	delete(r.clients, key)
}

func (r *ClientBox) Send(value []byte) {
    defer r.Unlock()
    r.Lock()
    for _,ch := range r.clients {
        log.Printf("Sending to %+v\n", ch.(*WWSs).name)
        ch.(*WWSs).output<-value
    }
}

type Handler struct {
	config *util.MzConfig
}

func NewHandler(config *util.MzConfig) *Handler {
	return &Handler{
		config: config,
	}
}

func (r *Handler) Index(resp http.ResponseWriter, req *http.Request) {
	log.Printf("Index...\n")
	resp.Write([]byte("Index"))
}

func (r *Handler) WSHandler(ws *websocket.Conn) {
    rnd := make([]byte, 4)
	rand.Read(rnd)
    name := hex.EncodeToString(rnd)

	sock := &WWSs{
		handler: r,
		name:    name,
        output: make(chan []byte),
	}
    Clients.Add(name, sock)
	sock.Run(ws)
}

type Upd struct {
    Message []byte `json:"message"`
}


func (r *Handler) Update(resp http.ResponseWriter, req *http.Request) {
    defer req.Body.Close()
    buf := make([]byte,100)
    req.Body.Read(buf)
    log.Printf("%s", buf)
    Clients.Send(buf)
}


// == socket
type WWS interface {
	Run(*websocket.Conn)
	Close() error
}

type WWSs struct {
	socket  *websocket.Conn
	handler *Handler
	name    string
	input   chan []byte
	quitter chan struct{}
	output  chan []byte
}

type replyType map[string]interface{}

func (r *WWSs) sniffer() (err error) {
	var raw = make([]byte, 1024)

	defer func() {
		log.Printf("Closing socket\n")
		close(r.quitter)
	}()

	for {
		err = websocket.Message.Receive(r.socket, &raw)
		if err != nil {
			switch {
			case err == io.EOF:
				log.Printf("Socket Closed remotely\n")
			default:
				log.Printf("Unknown error %s\n", err.Error())
			}
			return err
		}
		if len(raw) <= 0 {
			continue
		}
		log.Printf("Recv'd: %s", string(raw))
		r.input <- raw
	}
}

func (r WWSs) Close() error {
	close(r.quitter)
	return nil
}

func (r WWSs) Handler() *Handler {
	return r.handler
}

func (r WWSs) Run(sock *websocket.Conn) {
	r.input = make(chan []byte)
	r.quitter = make(chan struct{})

	defer func(sock WWS) {
		if rr := recover(); rr != nil {
			switch rr {
			case io.EOF:
				log.Printf("Closing socket")
			default:
				log.Printf("Unknown Socket error %s", rr.(error).Error())
			}
		}
		log.Printf("Cleaning up...\n")
		if r.quitter != nil {
			close(r.quitter)
		}
		return
	}(r)
    r.socket = sock

	go r.sniffer()
	for {
		select {
		case <-r.quitter:
			r.socket.Close()
			close(r.input)
			Clients.Del(r.name)
			r.quitter = nil
			return
		case input := <-r.input:
			msg := make(replyType)
			if err := json.Unmarshal(input, &msg); err != nil {
				log.Printf("Unparsable %s : %s\n", err.Error(), input)
			}
			log.Printf("### Got %+v\n", msg)
		case output := <-r.output:
			_, err := r.socket.Write(output)
			if err != nil {
				log.Printf("Could not write output %s\n", err.Error())
			}
		}
	}
}


