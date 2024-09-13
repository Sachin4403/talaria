package device

import (
	"fmt"
	"io"
	"time"

	"github.com/gorilla/websocket"
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

// Reader represents the read behavior of a device connection
type Reader interface {
	ReadMessage() (int, []byte, error)
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
}

// ReadCloser adds io.Closer behavior to Reader
type ReadCloser interface {
	io.Closer
	Reader
}

// Writer represents the write behavior of a device connection
type Writer interface {
	WriteMessage(int, []byte) error
	WritePreparedMessage(*websocket.PreparedMessage) error
	SetWriteDeadline(time.Time) error
}

// WriteCloser adds io.Closer behavior to Writer
type WriteCloser interface {
	io.Closer
	Writer
}

// Connection describes the set of behaviors for device connections used by this package.
type Connection interface {
	io.Closer
	Reader
	Writer
}

func zeroDeadline() time.Time {
	return time.Time{}
}

// NewDeadline creates a deadline closure given a timeout and a now function.
// If timeout is nonpositive, the return closure always returns zero time.
// If now is nil (and timeout is positive), then time.Now is used.
func NewDeadline(timeout time.Duration, now func() time.Time) func() time.Time {
	if timeout > 0 {
		if now == nil {
			now = time.Now
		}

		return func() time.Time {
			return now().Add(timeout)
		}
	}

	return zeroDeadline
}

// NewPinger creates a ping closure for the given connection.  Internally, a prepared message is created using the
// supplied data, and the given counter is incremented for each successful update of the write deadline.
func NewPinger(w Writer, pings xmetrics.Incrementer, data []byte, deadline func() time.Time) (func() error, error) {
	fmt.Println("Pinger data: ", string(data))
	pm, err := websocket.NewPreparedMessage(websocket.PingMessage, data)
	if err != nil {
		return nil, err
	}

	return func() error {
		err := w.SetWriteDeadline(deadline())
		if err != nil {
			fmt.Println("Error while setting deadline: ", err)
			return err
		}

		fmt.Println("Timestamp when writing prepared message:", time.Now(), " for device ID", string(data))
		err = w.WritePreparedMessage(pm)
		if err != nil {
			fmt.Println("Error while writing prepared message: ", err)
			return err
		}

		// only incrememt when the complete ping operation was successful
		pings.Inc()
		fmt.Println("Timestamp when ping was successful:", time.Now(), " for device ID", string(data))
		fmt.Println("Ping was successfull")
		return nil
	}, nil
}

// SetPongHandler establishes an instrumented pong handler for the given connection that enforces
// the given read timeout.
func SetPongHandler(r Reader, pongs xmetrics.Incrementer, deadline func() time.Time) {
	r.SetPongHandler(func(pongMessage string) error {
		fmt.Println("This is the pong message: ", pongMessage)
		fmt.Println("Timestamp in pong handler:", time.Now(), " for device ID", pongMessage)
		// increment up front, as this function is only called when a pong is actually received
		pongs.Inc()
		return r.SetReadDeadline(deadline())
	})
}

type instrumentedReader struct {
	ReadCloser
	statistics Statistics
}

func (ir *instrumentedReader) ReadMessage() (int, []byte, error) {
	messageType, data, err := ir.ReadCloser.ReadMessage()
	fmt.Println("Read message type: ", messageType)
	fmt.Println("Read message data: ", string(data))
	fmt.Println("Read message bytes received: ", len(data))
	if err == nil {
		ir.statistics.AddBytesReceived(len(data))
		ir.statistics.AddMessagesReceived(1)
	}

	return messageType, data, err
}

func InstrumentReader(r ReadCloser, s Statistics) ReadCloser {
	return &instrumentedReader{r, s}
}

type instrumentedWriter struct {
	WriteCloser
	statistics Statistics
}

func (iw *instrumentedWriter) WriteMessage(messageType int, data []byte) error {
	fmt.Println("Read message type: ", messageType)
	fmt.Println("Read message data: ", string(data))
	fmt.Println("Read message bytes received: ", len(data))
	err := iw.WriteCloser.WriteMessage(messageType, data)
	if err != nil {
		return err
	}

	iw.statistics.AddBytesSent(len(data))
	iw.statistics.AddMessagesSent(1)
	return nil
}

func (iw *instrumentedWriter) WritePreparedMessage(pm *websocket.PreparedMessage) error {
	err := iw.WriteCloser.WritePreparedMessage(pm)
	if err != nil {
		return err
	}

	// TODO: There isn't any way to obtain the length of a prepared message, so there's not a way to instrument it
	// at the moment
	iw.statistics.AddMessagesSent(1)
	return nil
}

func InstrumentWriter(w WriteCloser, s Statistics) WriteCloser {
	return &instrumentedWriter{w, s}
}
