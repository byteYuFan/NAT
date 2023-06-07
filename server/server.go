package main

import (
	"errors"
	"net"
	"sync"
)

var serverInstance *Server

// Server 服务端程序的实例
type Server struct {
	// CurrentConnInfo 全局的map里面存储的是目前有多少个内网客户端发送来了请求我们根据key int64 value "conn" 作为键值对存储
	CurrentConnInfo map[*net.TCPConn]int64
	// Mutex 保证并发安全的锁
	Mutex sync.RWMutex
	// Counter 目前服务器累计接收到了多少次连接
	Counter int64
	// TaskQueueSlice 工作队列
	TaskQueueSlice []*WorkerQueue
	// 最大连接数量
	MaxTCPConnSize int32
	// 最大连接数量
	MaxConnSize int32
	// ExposePort 服务端暴露端口
	ExposePort []int
	// TaskQueueBuffer 任务队列容量
	TaskQueueBufferSize int32
	// TaskQueueSize
	TaskQueueSize int32
	// ProcessingMap
	ProcessingMap map[string]*net.TCPConn
	// ListenerAndClientConn
	ListenerAndClientConn map[*net.TCPConn]*net.TCPListener
	// WorkerBuffer 整体工作队列的大小
	WorkerBuffer chan *net.TCPConn
	// 实际处理工作的数据结构
	ProcessWorker *Workers
	// 端口使用情况
	PortStatus map[int32]bool
}

func initServer() {
	serverInstance = &Server{
		CurrentConnInfo:       make(map[*net.TCPConn]int64),
		Mutex:                 sync.RWMutex{},
		Counter:               0,
		TaskQueueSlice:        nil,
		MaxTCPConnSize:        objectConfig.MaxTCPConnNum,
		MaxConnSize:           objectConfig.MaxConnNum,
		ExposePort:            objectConfig.ExposePort,
		TaskQueueBufferSize:   objectConfig.TaskQueueBufferSize,
		TaskQueueSize:         objectConfig.TaskQueueNum,
		ProcessingMap:         make(map[string]*net.TCPConn),
		ListenerAndClientConn: make(map[*net.TCPConn]*net.TCPListener),
		WorkerBuffer:          make(chan *net.TCPConn, objectConfig.MaxConnNum),
		ProcessWorker:         NewWorkers(),
		PortStatus:            make(map[int32]bool),
	}
	// 初始化队列
	serverInstance.TaskQueueSlice = make([]*WorkerQueue, serverInstance.MaxTCPConnSize)
	for i := 0; i < int(serverInstance.MaxTCPConnSize); i++ {
		serverInstance.TaskQueueSlice[i] = NewWorkerQueue(serverInstance.TaskQueueBufferSize, serverInstance.ExposePort[i])
	}
	// 初始化端口状态
	for i := 0; i < int(serverInstance.MaxTCPConnSize); i++ {
		serverInstance.PortStatus[int32(serverInstance.ExposePort[i])] = false
	}
}

// AddClientConn 添加客户端连接信息
func (s *Server) AddClientConn(conn *net.TCPConn) error {
	// 加锁
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	if int32(len(s.CurrentConnInfo)) > s.MaxConnSize {
		return errors.New("客户端请求数量太多")
	}
	s.CurrentConnInfo[conn] = s.Counter
	s.Counter++
	return nil
}

// RemoveClientConn 删除客户端连接信息
func (s *Server) RemoveClientConn(conn *net.TCPConn) {
	// 加锁
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	delete(s.CurrentConnInfo, conn)
}

// GetClientUid 获取客户端uid
func (s *Server) GetClientUid(conn *net.TCPConn) (int64, error) {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	if value, ok := s.CurrentConnInfo[conn]; ok {
		return value, nil
	} else {
		return -1, errors.New("错误的conn")
	}
}

// PushConnToTaskQueue 根据uid 将不同的连接推入到工作队列中去
func (s *Server) PushConnToTaskQueue(conn *net.TCPConn) error {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	uid, err := s.GetClientUid(conn)
	if err != nil {
		return err
	}
	s.TaskQueueSlice[uid%int64(s.TaskQueueSize)].Worker <- conn
	return nil
}

// PullConnFromTaskQueue 将conn从对应的工作队列中取出
func (s *Server) PullConnFromTaskQueue(conn *net.TCPConn) (*net.TCPConn, error) {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	uid, err := s.GetClientUid(conn)
	if err != nil {
		return nil, err
	}
	<-s.TaskQueueSlice[uid%int64(s.TaskQueueSize)].Worker
	return conn, nil
}

func (s *Server) GetConnPortByUID(uid int64) int {
	return s.TaskQueueSlice[uid%(int64(s.TaskQueueSize))].GetPort()
}

func (s *Server) AddListenerAndClient(l *net.TCPListener, c *net.TCPConn) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	s.ListenerAndClientConn[c] = l
}
func (s *Server) RemoveListenerAndClient(c *net.TCPConn) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()
	delete(s.ListenerAndClientConn, c)
}

func (s *Server) GetNumOfCurrentConn() int {
	s.Mutex.RLock()
	s.Mutex.RUnlock()
	return len(s.CurrentConnInfo)
}

func (s *Server) PortIsFull() bool {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	for _, v := range s.PortStatus {
		if v == false {
			return false
		}
	}
	return true
}

func (s *Server) GetPort() int32 {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	for k, v := range s.PortStatus {
		if v == true {
			continue
		} else {
			return k
		}
	}
	return -1
}
