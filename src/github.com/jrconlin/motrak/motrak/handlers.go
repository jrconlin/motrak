package motrak

import (
	"code.google.com/p/go.net/websocket"
	"github.com/jrconlin/motrak/util"

	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

var (
	Clients *ClientBox
)

type ClientBox struct {
	sync.RWMutex
	clients map[string]WWS
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

// == socket
type Sock interface {
	Close() error
	Write([]byte) (int, error)
}

type WWS interface {
	Run()
	Close() error
	Socket() Sock
}

type WWSs struct {
	socket  Sock
	handler *Handler
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
		err = websocket.Message.Receive(r.socket.(*websocket.Conn), &raw)
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

func (r *WWSs) Close() error {
	close(r.quitter)
	return nil
}

func (r *WWSs) Socket() Sock {
	return r.socket
}

func (r *WWSs) Handler() *Handler {
	return r.handler
}

func (r *WWSs) Run() {
	r.input = make(chan []byte)
	r.quitter = make(chan struct{})
	r.output = make(chan []byte)

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

	go r.sniffer()
	for {
		select {
		case <-r.quitter:
			r.socket.Close()
			close(r.input)
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

func (r *Handler) WSHandler(ws *websocket.Conn) {
	sock := &WWSs{
		handler: r,
	}
	sock.Run()
}
