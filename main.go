package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/apache/rocketmq-client-go/v2"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/apache/rocketmq-client-go/v2/producer"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/hooklift/gowsdl"
)

var default_port int = 8080

func main() {
	osTray := MakeTray()
	osTray.RunTray(onReady, onExit)
}

type CreateServerReq struct {
	Port        int               `json:"port"`
	Type        string            `json:"type"`
	Data        string            `json:"mockData"`
	ContentType string            `json:"contentType"`
	Headers     map[string]string `json:"headers"`
}

type ServerWrapper struct {
	Addr     string
	instance interface{}
}

func NewServerWrapper(addr string, instance interface{}) *ServerWrapper {
	return &ServerWrapper{Addr: addr, instance: instance}
}

func (server *ServerWrapper) ShutDown() {
	if httpServer, ok := server.instance.(*http.Server); ok {
		if err := httpServer.Shutdown(context.Background()); err != nil {
			fmt.Printf("Failed to stop HTTP server on port %v: %v\n", httpServer.Addr, err)
		}
	} else if socketServer, ok := server.instance.(net.Listener); ok {
		if err := socketServer.Close(); err != nil {
			fmt.Printf("Failed to stop socket server on port %d: %v\n", server.instance, err)
		}
	} else {
		fmt.Printf("Unknown server type for instance: %v\n", server.instance)
	}
}

// DynamicServer 结构体用于管理动态创建的服务器
type DynamicServer struct {
	servers       map[int]*ServerWrapper
	serverConfigs map[int]*CreateServerReq
	serversMutex  sync.Mutex
	wsConnections map[*websocket.Conn]map[int]bool
	wsConnMutex   sync.RWMutex
	upgrader      websocket.Upgrader
}

// WSMessage 定义了WebSocket消息的结构
type WSMessage struct {
	Action string      `json:"action"`
	Port   int         `json:"port"`
	Data   interface{} `json:"data,omitempty"`
	Run    bool        `json:"run"`
}

// NewDynamicServer 创建并初始化一个新的 DynamicServer 实例
func NewDynamicServer() *DynamicServer {
	return &DynamicServer{
		servers:       make(map[int]*ServerWrapper),
		serverConfigs: make(map[int]*CreateServerReq),
		wsConnections: make(map[*websocket.Conn]map[int]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func startServer() {
	gin.SetMode(gin.ReleaseMode)
	fmt.Printf("Server process ID: %d\n", os.Getpid())
	r := gin.Default()
	r.Static("/static", "./static")
	r.POST("/mockhttp", mockHttpRoute)
	r.POST("/http", httpRoute)
	r.POST("/mllp", mllpRoute)
	r.POST("/soap", soapRoute)
	r.POST("/wsdl-methods", wsdlMethods)
	r.POST("/rocketmq", rocketmqRoute)

	dynamicServer := NewDynamicServer()
	r.POST("/createserver", dynamicServer.CreateServer)
	r.GET("/ws", dynamicServer.HandleWebSocket)
	r.GET("/listservers", dynamicServer.ListServers)
	r.POST("/stopserver", dynamicServer.StopServer)
	r.GET("/getserver", dynamicServer.GetServerConfig)
	r.POST("/updateserver", dynamicServer.UpdateServer)
	port := flag.Int("port", default_port, "Port to run the server on")
	flag.Parse()
	default_port = *port

	for i := 0; i < 10; i++ {
		port := fmt.Sprintf(":%d", default_port)
		fmt.Printf("Trying to start server on port %s\n", port)
		err := r.Run(port)
		if err == nil {
			fmt.Printf("Server started successfully on port %d\n", default_port)
			break
		} else {
			fmt.Printf("Failed to start server on port %s: %v\n", port, err)
		}
		default_port++
	}
}

func onReady() {
	startServer()
}

func onExit() {
	fmt.Println("Exiting...")
	os.Exit(0)
}

func mockHttpRoute(c *gin.Context) {
	fmt.Println("---header ", c.Request.Header)
	body, _ := io.ReadAll(c.Request.Body)
	fmt.Println("---request.data ", string(body))

	sendApp := c.GetHeader("CamelMllpSendingApplication")
	uuidStr := uuid.New().String()

	if sendApp != "" {
		responseBody := strings.ReplaceAll(string(body), "PACS", "xxxx")
		c.Header("traceId111", uuidStr)
		c.Header("time", fmt.Sprintf("%f", float64(time.Now().UnixNano())/1e9))
		c.Header("CamelMllpSendingApplication", sendApp+"_ABC")
		c.Data(http.StatusOK, "application/octet-stream", []byte(responseBody))
	} else {
		fmt.Println("---raw data:", string(body))
		c.Data(http.StatusOK, "application/octet-stream", body)
	}
}

// httpRoute 处理HTTP请求。
// 它解析传入的JSON数据，提取请求头、方法、URL和请求体，
// 并使用这些信息构造一个新的HTTP请求。
// 然后，它发送请求并读取服务器的响应，将响应返回给客户端。
func httpRoute(c *gin.Context) {
	var data map[string]interface{}
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": err.Error(), "resp": ""})
		return
	}

	header := make(http.Header)
	if reqHeaders, ok := data["req_headers"].(map[string]interface{}); ok {
		for k, v := range reqHeaders {
			header.Set(k, fmt.Sprint(v))
		}
	}

	method := data["method"].(string)
	url := data["url"].(string)
	reqBody := []byte(data["req"].(string))

	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}
	req.Header = header

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var jsonResp interface{}
	if err := json.Unmarshal(body, &jsonResp); err != nil {
		jsonResp = string(body)
	}

	c.JSON(http.StatusOK, gin.H{"status": resp.StatusCode, "errMsg": "", "resp": jsonResp})
}

// mllpRoute handles MLLP requests.
// It parses the incoming JSON data, extracts the host and port from the URL,
// and sends the request to the specified URL using the MLLP protocol.
// The response from the server is then read and returned in the response body.``

func mllpRoute(c *gin.Context) {
	var data map[string]string
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": err.Error(), "resp": ""})
		return
	}

	host, port, found := strings.Cut(data["url"], ":")
	if !found {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": "Invalid URL format", "resp": ""})
		return
	}

	response, err := sendMllpMessage(host, port, data["req"])
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "errMsg": "", "resp": string(response)})
}

// soapRoute handles SOAP requests.
// It parses the incoming JSON data, constructs a new HTTP POST request with the SOAPAction and Content-Type headers set,
// and sends the request to the specified URL.
// The response from the server is then read and returned in the response body.

func soapRoute(c *gin.Context) {
	var data map[string]string
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": err.Error(), "resp": ""})
		return
	}

	url := data["url"]
	payload := data["req"]
	fmt.Println("--soap ", payload)

	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}

	req.Header.Set("SOAPAction", "")
	req.Header.Set("Content-Type", "application/xml;charset=UTF-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.JSON(http.StatusOK, gin.H{"status": resp.StatusCode, "errMsg": "", "resp": string(body)})
}

// sendMllpMessage sends an HL7 message using the Minimal Lower Layer Protocol (MLLP).
// It constructs the message by wrapping the provided message string with MLLP start and end blocks.
// The function connects to the specified host and port, sends the constructed message, and reads the response.
// Parameters:
// - host: The host to connect to.
// - port: The port to connect to on the host.
// - message: The HL7 message to send.
// Returns:
// - []byte: The response from the server, with MLLP start and end blocks removed and line endings adjusted.
// - error: An error if any occurred during the process.

func sendMllpMessage(host, port, message string) ([]byte, error) {
	startBlock := []byte{0x0b}
	endBlock := []byte{0x1c}
	endData := []byte{0x0d}

	conn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := strings.ReplaceAll(message, "\n", "\r")
	_, err = conn.Write(append(append(append(startBlock, []byte(req)...), endBlock...), endData...))
	if err != nil {
		return nil, err
	}

	data := make([]byte, 10240)
	n, err := conn.Read(data)
	if err != nil {
		return nil, err
	}

	data = data[1 : n-2]
	return []byte(strings.ReplaceAll(string(data), "\r", "\r\n")), nil
}

// 封装一个方法，从interface转换为指定类型的值，如将interface{} 转换为 string 类型
func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func ConvertTo[T any](value interface{}) T {
	val, ok := value.(T)
	if !ok {
		fmt.Println("转换失败")
		return val
	}
	return val
}

type MqMsg struct {
	Topic    string                 `json:"topic"`
	Url      string                 `json:"url"`
	Body     map[string]interface{} `json:"body"`
	RetryNum int                    `json:"retry_num"`
}

// 接收http请求，发送指定消息到rocketmq
func rocketmqRoute(c *gin.Context) {
	var data MqMsg
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": err.Error(), "resp": ""})
		return
	}

	parts := strings.Split(data.Url, ":")
	if len(parts) != 2 {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": "Invalid URL format", "resp": ""})
		return
	}

	host, port := parts[0], parts[1]
	fmt.Println("host ", host, " port ", port)

	// 如果有body参数，则解析body参数
	if data.Body == nil {
		fmt.Printf("error unmarshalling message body: \n")
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": "Invalid body", "resp": ""})
		return
	}

	body, err := json.Marshal(data.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": "Invalid body", "resp": ""})
		return
	}

	header := ConvertTo[map[string]interface{}](data.Body["header"])
	operType := header["operType"].(string)
	key := topip_keymap[operType]
	fmt.Printf("mashal body is %s\n", string(body))
	response, err := sendRocketmqMessage(host, port, data.Topic, string(body), key, data.RetryNum)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": err.Error(), "resp": ""})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "errMsg": "", "resp": response})
}

var topip_keymap map[string]string = map[string]string{
	"0": "ADD",
	"1": "UPDATE",
	"2": "DELETE",
}

// 调用rocketmq 客户端发送消息
func sendRocketmqMessage(host, port, topic, message, key string, retry int) (*primitive.SendResult, error) {
	fmt.Printf("try to send rocketmq message, host: %s, port: %s, topic: %s, message: %s, retry: %d\n", host, port, topic, message, retry)
	endpoint := fmt.Sprintf("%s:%s", host, port)
	p, err := rocketmq.NewProducer(
		producer.WithNameServer([]string{endpoint}),
		producer.WithRetry(retry),
	)

	if err != nil {
		fmt.Printf("create producer error %v\n", err)
		return nil, err
	}

	err = p.Start()
	if err != nil {
		fmt.Println("start producer error", err)
		return nil, err
	}

	msg := &primitive.Message{
		Topic: topic,
		Body:  []byte(message),
	}

	//使用 msg的withKeys方法设置kemsg.WithKeys([]string{"your_message_key"})y
	msg = msg.WithKeys([]string{key})

	ret, err := p.SendSync(context.Background(), msg)
	if err != nil {
		fmt.Println("send message error", err)
		return nil, err
	}

	fmt.Printf("send to topic %s, result: %s\n", topic, ret.String())
	err = p.Shutdown()

	if err != nil {
		fmt.Println("shutdown producer error", err)
		return nil, err
	}
	return ret, nil
}

// 补充中文注释
func wsdlMethods(c *gin.Context) {
	var data struct {
		URL string `json:"url"`
	}

	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "errMsg": err.Error(), "resp": ""})
		return
	}

	wsdlURL := data.URL

	// 获取 WSDL 内容
	resp, err := http.Get(wsdlURL)
	fmt.Println("wsdl url", wsdlURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": fmt.Sprintf("Failed to fetch WSDL: %v", err), "resp": ""})
		return
	}
	defer resp.Body.Close()

	wsdlContent, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": fmt.Sprintf("Failed to read WSDL content: %v", err), "resp": ""})
		return
	}

	wsdl := gowsdl.WSDL{}
	// 解析 WSDL
	err = xml.Unmarshal(wsdlContent, &wsdl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "errMsg": fmt.Sprintf("Failed to parse WSDL: %v", err), "resp": ""})
		return
	}

	// 提取方法名
	var methods []string
	for _, binding := range wsdl.Binding {
		for _, operation := range binding.Operations {
			methods = append(methods, operation.Name)
		}
	}

	// 返回结果
	c.JSON(http.StatusOK, gin.H{"status": 200, "errMsg": "", "resp": methods})
}

func handleServerCreationResult(req *CreateServerReq, createChan chan interface{}) error {
	ready := <-createChan
	switch v := ready.(type) {
	case string:
		return fmt.Errorf("failed to start %s server on port %d: %s", req.Type, req.Port, v)
	case bool:
		if !v {
			return fmt.Errorf("failed to start %s server on port %d", req.Type, req.Port)
		}
	default:
		return fmt.Errorf("unknown error occurred while starting the server")
	}
	return nil
}

// CreateServer 处理创建新服务器的请求
func (ds *DynamicServer) CreateServer(c *gin.Context) {

	var req CreateServerReq

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Data == "" {
		req.Data = "default mock data"
	}

	if _, exists := ds.servers[req.Port]; exists {
		c.JSON(http.StatusInternalServerError, gin.H{"message": fmt.Sprintf("Server already exists on this port %d", req.Port)})
		return
	}

	createChan := make(chan interface{})
	defer close(createChan)
	switch req.Type {
	case "http":
		go ds.startHTTPServer(&req, createChan)
	case "hl7":
		go ds.startMllpServer(&req, createChan)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unsupported server type"})
		return
	}

	if err := handleServerCreationResult(&req, createChan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s server started on port %d", req.Type, req.Port)})
	}
}

// startMllpServer 启动一个新的 MLLP 服务器
func (ds *DynamicServer) startMllpServer(req *CreateServerReq, readyChan chan interface{}) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(req.Port))
	if err != nil {
		readyChan <- err.Error()
		return
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Printf("Error accepting connection: %v\n", err)
				break
			}
			go ds.handleMllpConnection(conn, req)
		}
	}()

	ds.serversMutex.Lock()
	ds.servers[req.Port] = NewServerWrapper(listener.Addr().String(), listener)
	ds.serverConfigs[req.Port] = req
	ds.serversMutex.Unlock()

	readyChan <- true
	fmt.Printf("MLLP server listening on port %d\n", req.Port)
}

func (ds *DynamicServer) handleMllpConnection(conn net.Conn, req *CreateServerReq) {
	defer conn.Close()

	buffer := make([]byte, 1024)
	startBlock := []byte{0x0b}
	endBlock := []byte{0x1c, 0x0d}

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("Error reading from connection: %v\n", err)
			}
			return
		}

		// 检查是否是有效的 MLLP 消息
		if n < 3 || buffer[0] != startBlock[0] || buffer[n-2] != endBlock[0] || buffer[n-1] != endBlock[1] {
			fmt.Println("Invalid MLLP message")
			continue
		}

		// 提取消息内容
		message := buffer[1 : n-2]

		msg := WSMessage{
			Action: "request",
			Port:   req.Port,
			Run:    true,
			Data: map[string]interface{}{
				"method":     "HL7",
				"body":       string(message),
				"mockData":   req.Data,
				"mockHeader": req.Headers,
			},
		}

		ds.broadcastToWebSockets(msg)

		// 发送响应
		response := append(startBlock, []byte(req.Data)...)
		response = append(response, endBlock...)
		_, err = conn.Write(response)
		if err != nil {
			fmt.Printf("Error sending response: %v\n", err)
			return
		}
	}
}

// startHTTPServer 启动一个新的 HTTP 服务器
func (ds *DynamicServer) startHTTPServer(req *CreateServerReq, readyChan chan interface{}) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		msg := WSMessage{
			Action: "request",
			Port:   req.Port,
			Run:    true,
			Data: map[string]interface{}{
				"method":     r.Method,
				"url":        r.URL.String(),
				"headers":    r.Header,
				"body":       string(body),
				"mockData":   req.Data,
				"mockHeader": req.Headers,
			},
		}
		//根据header中的X-Delay字段模拟指定的延迟

		if delay, ok := req.Headers["X-Delay"]; ok {
			//将delay字段转换为整数
			delayInt, err := strconv.Atoi(delay)
			if err != nil {
				fmt.Println("转换失败")
			} else {
				time.Sleep(time.Duration(delayInt) * time.Millisecond)
			}
		}

		ds.broadcastToWebSockets(msg)
		for key, value := range req.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(http.StatusOK)

		w.Write([]byte(req.Data))
	})

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(req.Port),
		Handler: mux,
	}
	fmt.Println("try to listenadnserve on ", req.Port)

	serveReady := make(chan interface{})
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			serveReady <- err.Error()
		}
		close(serveReady)
	}()

	select {
	case err := <-serveReady:
		readyChan <- err
		fmt.Printf("Error starting server on port %d: %v\n", req.Port, err)
	case <-time.After(100 * time.Millisecond):
		readyChan <- true
		ds.serversMutex.Lock()
		ds.servers[req.Port] = NewServerWrapper(server.Addr, server)
		ds.serverConfigs[req.Port] = req
		ds.serversMutex.Unlock()
		fmt.Printf("server listen on port %d n", req.Port)
	}
}

// HandleWebSocket 处理 WebSocket 连接
func (ds *DynamicServer) HandleWebSocket(c *gin.Context) {
	conn, err := ds.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Printf("Failed to upgrade to WebSocket: %v\n", err)
		return
	}

	ds.wsConnMutex.Lock()
	ds.wsConnections[conn] = make(map[int]bool)
	ds.wsConnMutex.Unlock()

	defer func() {
		ds.wsConnMutex.Lock()
		delete(ds.wsConnections, conn)
		ds.wsConnMutex.Unlock()
		conn.Close()
	}()

	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			break
		}

		switch msg.Action {
		case "subscribe":
			ds.wsConnMutex.Lock()
			ds.wsConnections[conn][msg.Port] = true
			ds.wsConnMutex.Unlock()
			conn.WriteJSON(WSMessage{
				Action: "subok",
				Port:   msg.Port,
				Run:    true,
			})
		case "unsubscribe":
			ds.wsConnMutex.Lock()
			delete(ds.wsConnections[conn], msg.Port)
			ds.wsConnMutex.Unlock()
		}
	}
}

// broadcastToWebSockets 向订阅的 WebSocket 连接广播消息
func (ds *DynamicServer) broadcastToWebSockets(msg WSMessage) {
	ds.wsConnMutex.Lock()
	defer ds.wsConnMutex.Unlock()
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		fmt.Printf("Error marshalling message: %v\n", err)
		return
	}

	for conn, ports := range ds.wsConnections {
		if ports[msg.Port] {
			err := conn.WriteMessage(websocket.TextMessage, jsonMsg)
			if err != nil {
				fmt.Printf("Error sending WebSocket message: %v\n", err)
				conn.Close()
				delete(ds.wsConnections, conn)
			}
		}
	}
}

// ListServers 列出所有动态创建的服务器
func (ds *DynamicServer) ListServers(c *gin.Context) {
	ds.serversMutex.Lock()
	defer ds.serversMutex.Unlock()

	var serverList []map[string]interface{}
	for port, server := range ds.servers {
		config := ds.serverConfigs[port]
		serverList = append(serverList, map[string]interface{}{
			"port":   port,
			"status": "Running",
			"addr":   server.Addr,
			"type":   config.Type,
		})
	}

	c.JSON(http.StatusOK, gin.H{"servers": serverList})
}

// StopServer 关闭指定端口的服务器
func (ds *DynamicServer) StopServer(c *gin.Context) {
	var req struct {
		Port int `json:"port"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ds.serversMutex.Lock()
	defer ds.serversMutex.Unlock()

	server, exists := ds.servers[req.Port]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found on this port"})
		return
	}

	ds.broadcastToWebSockets(WSMessage{
		Action: "shutdown",
		Port:   req.Port,
		Run:    false,
	})
	server.ShutDown()

	delete(ds.servers, req.Port)

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Server on port %d has been stopped", req.Port)})
}

// UpdateServer 更新指定端口的服务器配置
func (ds *DynamicServer) UpdateServer(c *gin.Context) {
	var req CreateServerReq

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ds.serversMutex.Lock()
	defer ds.serversMutex.Unlock()

	serverConfig, exists := ds.serverConfigs[req.Port]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found on this port"})
		return
	}

	// 更新除了端口号之外的配置
	if req.Data != "" {
		serverConfig.Data = req.Data
	}
	if req.Headers != nil {
		serverConfig.Headers = req.Headers
	}

	fmt.Println("serverConfig.Data = ", req.Data)

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Server on port %d has been updated", req.Port)})
}

// GetServerConfig 获取指定端口的服务器配置信息
func (ds *DynamicServer) GetServerConfig(c *gin.Context) {
	portStr := c.Query("port")
	fmt.Println("request port ", portStr)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid port number"})
		return
	}

	ds.serversMutex.Lock()
	defer ds.serversMutex.Unlock()

	serverConfig, exists := ds.serverConfigs[port]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Server not found on this port"})
		return
	}

	fmt.Println("")

	c.JSON(http.StatusOK, serverConfig)
}
