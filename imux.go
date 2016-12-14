package imux

import (
	log "github.com/Sirupsen/logrus"
	"github.com/hkparker/TLJ"
	"net"
	"reflect"
)

var write_queues = make(map[string]WriteQueue)

// Provide a net.Listener, for which any accepted sockets will have their data
// inverse multiplexed to the destination defined in the ConnectionPool.
func OneToMany(listener net.Listener, destination ConnectionPool) error {
	// In an infinite loop, accept new connections to this listener
	// and chunk any data written to the accepted sockets into the
	// destination ConnectionPool.
	for {
		socket, err := listener.Accept()
		if err != nil {
			log.WithFields(log.Fields{}).Error(err)
			return err
		}
		socket_id := "" // generate an ID for this socket

		// Any chunks sent back from the destination into sockets
		// in the connection pool need to be written to the correct
		// socket.  Specify in this ConnectionPool that this newly
		// created ID corresponds to this newly accepted socket.
		destination.chunksBackTo(socket, socket_id)

		// In an infinite loop in a goroutine, read data from the
		// socket and send chunks to the destination ConnectionPool
		go func(socket net.Conn, id string) {
			sequence_number := 0
			for {
				chunk_data := make([]byte, destination.ChunkSize)
				if _, err := socket.Read(chunk_data); err != nil {
					destination.end(id)
					log.WithFields(log.Fields{}).Info(err)
					return
				} else {
					destination.chunks <- Chunk{
						SocketID:   id,
						SequenceID: sequence_number,
						Data:       chunk_data,
					}
				}
				sequence_number = sequence_number + 1
			}
		}(socket, socket_id)
	}
	return nil
}

// Create a new TLJ server to accept chunks from anywhere
// and order them, writing them to corresponding sockets.
func ManyToOne(listener net.Listener, destination string) {
	tlj_server := tlj.NewServer(listener, tag_socket, type_store())
	tlj_server.Accept("all", reflect.TypeOf(Chunk{}), func(iface interface{}, context tlj.TLJContext) {
		if chunk, ok := iface.(*Chunk); ok {
			queue, present := write_queues[chunk.SocketID]
			if !present {
				queue = OpenWriteQueue(destination)
			}
			queue.WriteChunk(chunk.SequenceID, chunk.Data)
			log.WithFields(log.Fields{}).Info(chunk.SocketID)
		}
	})
}