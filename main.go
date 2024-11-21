package main

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

type ChatRoom struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
}

func newChatRoom() *ChatRoom {
	return &ChatRoom{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

func (c *ChatRoom) run() {
	for {
		select {
		case client := <-c.register:
			c.clients[client] = true
			fmt.Println("New client connected")
		case client := <-c.unregister:
			delete(c.clients, client)
			close(client.send)
			fmt.Println("Client disconnected")
		case message := <-c.broadcast:
			for client := range c.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(c.clients, client)
				}
			}
		}
	}
}

func (c *Client) readMessages() {
	defer func() {
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// Broadcast message to all clients
		c.conn.WriteMessage(websocket.TextMessage, message)
	}
}

func (c *Client) writeMessages() {
	defer func() {
		c.conn.Close()
	}()
	for message := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			break
		}
	}
}

func handleWebSocket(c *gin.Context, chatRoom *ChatRoom) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("Error upgrading to WebSocket:", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte),
	}

	chatRoom.register <- client
	defer func() {
		chatRoom.unregister <- client
	}()

	go client.readMessages()
	go client.writeMessages()

	for {
		// Wait for message from client to broadcast
		messageType, p, err := conn.ReadMessage()
		if err != nil || messageType != websocket.TextMessage {
			break
		}
		chatRoom.broadcast <- p
	}
}

func serveHomePage(c *gin.Context) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "Template parse error: %v", err)
		return
	}
	tmpl.Execute(c.Writer, nil)
}

func main() {
	chatRoom := newChatRoom()
	go chatRoom.run()

	router := gin.Default()
	router.LoadHTMLGlob("templates/*")

	// Serve WebSocket
	router.GET("/ws", func(c *gin.Context) {
		handleWebSocket(c, chatRoom)
	})

	// Serve HomePage with chat template
	router.GET("/", serveHomePage)

	// Start the server
	router.Run(":8080")
}
