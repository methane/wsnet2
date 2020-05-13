package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"

	"wsnet2/binary"
	"wsnet2/pb"
)

func main() {
	conn, err := grpc.Dial("127.0.0.1:19000", grpc.WithInsecure())
	if err != nil {
		log.Fatal("client connection error:", err)
	}
	defer conn.Close()

	client := pb.NewGameClient(conn)
	req := &pb.CreateRoomReq{
		AppId: "testapp",
		RoomOption: &pb.RoomOption{
			Visible:   true,
			Watchable: true,
			LogLevel:  4,
		},
		MasterInfo: &pb.ClientInfo{
			Id: "11111",
		},
	}

	res, err := client.Create(context.TODO(), req)
	if err != nil {
		fmt.Printf("create room error: %v", err)
	}

	url := fmt.Sprintf("ws://localhost:8000/room/%s", res.RoomInfo.Id)
	hdr := http.Header{}
	hdr.Add("X-Wsnet-App", "testapp")
	hdr.Add("X-Wsnet-User", "11111")
	hdr.Add("X-Wsnet-LastEventSeq", "0")

	d := websocket.Dialer{
		Subprotocols:    []string{},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	ws, res2, err := d.Dial(url, hdr)
	if err != nil {
		fmt.Printf("dial error: %v, %v\n", res2, err)
		return
	}
	fmt.Println("response:", res2)

	done := make(chan bool)
	go eventloop(ws, done)

	go func() {
		time.Sleep(time.Second)
		ws.Close()
	}()

	<-done

	time.Sleep(3 * time.Second)
	fmt.Println("reconnect test")
	//	hdr.Set("X-Wsnet-LastEventSeq", "1")
	ws, res2, err = d.Dial(url, hdr)
	if err != nil {
		fmt.Printf("dial error: %v, %v\n", res2, err)
		return
	}
	fmt.Println("response:", res2)

	done = make(chan bool)
	go eventloop(ws, done)

	go func() {
		time.Sleep(time.Second)
		ws.Close()
	}()

	<-done
}

func eventloop(ws *websocket.Conn, done chan bool) {
	defer close(done)
	for {
		t, b, err := ws.ReadMessage()
		if err != nil {
			fmt.Printf("ReadMessage error: %v\n", err)
			return
		}

		switch ty := binary.EvType(b[0]); ty {
		case binary.EvTypeJoined:
			seqnum := (int(b[1]) << 24) + (int(b[2]) << 16) + (int(b[3]) << 8) + int(b[4])
			namelen := int(b[6])
			name := string(b[7 : 7+namelen])
			props := b[7+namelen:]
			fmt.Printf("%s: %v %#v, %v, %v\n", ty, seqnum, name, props, b)
		default:
			fmt.Printf("ReadMessage: %v, %v\n", t, b)
		}
	}
}
