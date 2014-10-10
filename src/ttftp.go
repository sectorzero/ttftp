package main

// TODO 
// - File storage
// - Fix for sending last ack upon storing file
// - Endianess 

import(
    "bytes"
    "crypto/rand"
    "encoding/base64"
    "encoding/binary"
    "fmt"
    "io"
    "log"
    "math/big"
    "net"
    "os"
    "strconv"
    "sync"
)

const(
    control_port string = "localhost:9991"
    chunk_sz int = 512
    tftp_data_header_bytes int = 4
)

// ---------------------------------
// Utilities
// ---------------------------------
func chk_err(err error) {
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
        os.Exit(1)
    }
}

func trace(format string, v ...interface{}) {
    log.Printf(format, v...)
}

func get_random_tid() (tid_port string) {
    buf := new(bytes.Buffer)
    buf.WriteString(":")
    max := big.NewInt(5000);
    portrange, _ := rand.Int(rand.Reader, max)
    port := int(portrange.Uint64()) + 10000
    buf.WriteString(strconv.Itoa(port))
    return buf.String()
}

// ---------------------------------
// TFTP Protocol Encoding/Decoding
// ---------------------------------
type Message struct {
    opcode uint16
    key string
    payload [chunk_sz]byte
    block uint16
    errcode uint16
    errmsg string
    sz int
}

func (m Message) String() (string) {
    buf := new(bytes.Buffer)

    buf.WriteString("[ ")
    if m.opcode == 1 {
        buf.WriteString("<")
        buf.WriteString("WRQ")
        buf.WriteString(">")
        buf.WriteString(" Key=")
        buf.WriteString(m.key)
    } else if m.opcode == 2 {
        buf.WriteString("<")
        buf.WriteString("RRQ")
        buf.WriteString(">")
        buf.WriteString(" ")
        buf.WriteString(m.key)
    } else if m.opcode == 3 {
        buf.WriteString("<")
        buf.WriteString("DATA")
        buf.WriteString(">")
        buf.WriteString(" Block=")
        buf.WriteString(strconv.Itoa(int(m.block)))
        buf.WriteString(" PayloadSz=")
        buf.WriteString(strconv.Itoa(int(m.sz)))
    } else if m.opcode == 4 {
        buf.WriteString("<")
        buf.WriteString("ACK")
        buf.WriteString(">")
        buf.WriteString(" Block=")
        buf.WriteString(strconv.Itoa(int(m.block)))
    } else if m.opcode == 5 {
        buf.WriteString("<")
        buf.WriteString("ERR")
        buf.WriteString(">")
    } else {
        // Ignore
    }
    buf.WriteString(" ]")
    return buf.String()
}
    
func Decode(buf *bytes.Buffer) (m *Message) {
    m = new(Message)

    var opcode uint16
    binary.Read(buf, binary.BigEndian, &opcode);
    m.opcode = opcode
    if opcode == 1 {
        key, _ := buf.ReadString(byte(0));
        m.key = key
    } else if opcode == 2 {
        key, _ := buf.ReadString(byte(0));
        m.key = key
    } else if opcode == 3 {
        err := binary.Read(buf, binary.BigEndian, &m.block);
        chk_err(err)
        sz, err := buf.Read(m.payload[0:chunk_sz])
        if err != nil && err == io.EOF {
            m.sz = 0
        } else {
            chk_err(err)
            m.sz = sz
        }
    } else if opcode == 4 {
        binary.Read(buf, binary.BigEndian, &m.block);
    } else if opcode == 5 {
        errmsg, _ := buf.ReadString(byte(0));
        m.errmsg = errmsg
    } else {
        errmsg, _ := buf.ReadString(byte(0));
        m.errmsg = errmsg
    }

    return m
}

func Encode(m *Message) (buf *bytes.Buffer) {
    buf = new(bytes.Buffer)

    opcode := m.opcode
    binary.Write(buf, binary.BigEndian, uint16(m.opcode))
    if opcode == 1 {
        _, err := buf.Write([]byte(m.key))
        chk_err(err)
    } else if opcode == 2 {
        _, err := buf.Write([]byte(m.key))
        chk_err(err)
    } else if opcode == 3 {
        err := binary.Write(buf, binary.BigEndian, uint16(m.block))
        chk_err(err)
        _, err = buf.Write([]byte(m.payload[0:m.sz]))
        chk_err(err)
    } else if opcode == 4 {
        err := binary.Write(buf, binary.BigEndian, uint16(m.block))
        chk_err(err)
    } else if opcode == 5 {
        _, err := buf.Write([]byte("NODATA"))
        chk_err(err)
    } else {
        _, err := buf.Write([]byte("NODATA"))
        chk_err(err)
    }

    return buf
}

func testCodec() {
    // encode
    msg := new(Message)
    msg.opcode = 3
    msg.block = 213
    payload :=  "asdfaksdjflkasjdfjaslkdfjlaksdaadsfa"
    copy(msg.payload[:], payload)
    msg.sz = len(payload)
    encoded := Encode(msg)
    fmt.Println(encoded.Bytes())

    // decode
    decoded := Decode(encoded)
    fmt.Println(decoded.String())
}

// ---------------------------------
// TFTP Control Service
// ---------------------------------
func main() {
    // Control Server UDP Socket
    serveraddr, err := net.ResolveUDPAddr("udp", control_port)
    chk_err(err)
    serverconn, err := net.ListenUDP("udp", serveraddr)
    chk_err(err)

    // < TESTING MESSAGES >
    // go routine to create and send a message to this server
    go write_file("hola", 513)
    go read_file("hola")

    // Control Loop
    for {
        // == recvmsg == ( IO BLOCK )
        var buffer [1500]byte;
        n, clientaddr, err := serverconn.ReadFromUDP(buffer[0:])
        chk_err(err)
        trace("[SERVER] <read> : data=%s, bytes=%d, src=%s\n", string(buffer[0:n]), n, clientaddr.String())

        // decode the message
        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[SERVER] <message-in>:%s\n", datain.String())

        // orchestrate
        if datain.opcode == 1 {
            go wrq_session(datain, clientaddr)
        } else if datain.opcode == 2 {
            go rrq_session(datain, clientaddr)
        } else {
            trace("[SERVER] %s\n", "Invalid Request For Control Loop")
        }
    }
}

// ---------------------------------
// WRQ Session Handler
// ---------------------------------
type FileTransferStateIn struct {
    buf bytes.Buffer
    last_block_received uint16
}

func wrq_session(m *Message, clientaddr *net.UDPAddr) {
    trace("[WRQ-HANDLER] src=%s message-in=%s\n", clientaddr.String(), m.String())

    // 1. bind a new udp socket ( ListenUDP ) this is our new 'endpoint' for the session
    tid := get_random_tid()
    sessionaddr, err := net.ResolveUDPAddr("udp", tid)
    chk_err(err)
    sessionconn, err := net.ListenUDP("udp", sessionaddr)
    chk_err(err)

    // 2. send the initial ACK for WRQ transfer initiate
    first_ack := new(Message)
    first_ack.opcode = 4
    first_ack.block = 0
    encoded_first_ack := Encode(first_ack)
    n, err := sessionconn.WriteToUDP(encoded_first_ack.Bytes(), clientaddr) 
    chk_err(err)
    trace("[WRQ] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", first_ack.String(), n, sessionaddr.String(), clientaddr.String());

    // 3. Read DATA Blocks
    transfer_state := new(FileTransferStateIn)
    completed := false
    for {
        trace("[WRQ] %s\n", "Waiting on WRQ session loop")

        // == recvmsg == ( IO BLOCK : wait for data packets )
        var buffer [1500]byte;
        received_bytes, clientaddr, err := sessionconn.ReadFromUDP(buffer[0:])
        chk_err(err)
        trace("[WRQ] <read> : data=%s, bytes=%d, src=%s\n", base64.URLEncoding.EncodeToString(buffer[0:received_bytes]), received_bytes, clientaddr.String())

        // [TODO] validate clientaddr to ensure no cross-talk among sessions ( ignoring for now )

        // decode the packet
        datain := Decode(bytes.NewBuffer(buffer[0:received_bytes]))
        trace("[%s] <message-in>:%s\n", "WRQ", datain.String())

        // collect the data
        if datain.opcode == 3 {
            trace("[%s] %s\n", "WRQ", "GOT DATA!!")

            if datain.block != transfer_state.last_block_received + 1 {
                trace("[WRQ] Block Sequence Error, Actual=%d, Expected=%d, Message=%s\n", datain.block, transfer_state.last_block_received + 1, datain.String())

                // send ack
                er := new(Message)
                er.opcode = 5
                er.errmsg = "Invalid Block Sequence"
                encoded_er := Encode(er)
                n, err := sessionconn.WriteToUDP(encoded_er.Bytes(), clientaddr) 
                chk_err(err)
                trace("[%s] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", er.String(), n, sessionaddr.String(), clientaddr.String());

                trace("[WRQ] Terminating WRQ Session")
                break;
            }

            // append/store data in temp buffer
            transfer_state.buf.Write(datain.payload[:])
            transfer_state.last_block_received = datain.block

            // send ack
            ack := new(Message)
            ack.opcode = 4
            ack.block = datain.block
            encoded_ack := Encode(ack)
            n, err := sessionconn.WriteToUDP(encoded_ack.Bytes(), clientaddr) 
            chk_err(err)
            trace("[%s] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", ack.String(), n, sessionaddr.String(), clientaddr.String());

            // If EOF, complete the file storage transaction
            if received_bytes < ( chunk_sz + tftp_data_header_bytes ) {
                completed = true
                break
            }
        } else {
            trace("[%s] %s\n", "WRQ", "Invalid Request For Control Loop")
        }
    }

    if completed == true {
        // store the file
        trace("[WRQ] data receieved fully, storing file Key=%s\n", m.key)
        // complete file PUT transaction
        trace("[WRQ] COMPLETED, File=%s\n", m.key)
        // last ack can signify error if unable to store
    }
}

// ---------------------------------
// RRQ Session Handler
// ---------------------------------
func rrq_session(m *Message, clientaddr *net.UDPAddr) {
    trace("[RRQ-HANDLER] src=%s message-in=%s\n", clientaddr.String(), m.String())

    // 1. bind a new udp socket ( ListenUDP ) this is our new 'endpoint' for the session
    tid := get_random_tid()
    sessionaddr, err := net.ResolveUDPAddr("udp", tid)
    chk_err(err)
    sessionconn, err := net.ListenUDP("udp", sessionaddr)
    chk_err(err)

    // validate if file is present else respond with error
    payload_sz := chunk_sz
    payload := generate_random_bytes(payload_sz)
    key := "key-1"

    // send data
    completed := false
    send_zero_eof := false
    remaining := payload_sz;
    st := 0
    en := 0
    var block uint16 = 0
    for {
        block = block + 1

        dataout := new(Message)
        dataout.opcode = 3
        dataout.block = block

        // determine chunk to send
        if send_zero_eof == true {
            dataout.sz = 0
            var emptybuf [0]byte
            copy(dataout.payload[0:], emptybuf[0:0])
            trace("[RRQ] preparing to send zero packet eof : Block=%d\n", block)
        } else {
            st = en
            if (st + chunk_sz) < (st + remaining) {
                en = st + chunk_sz
            } else {
                en = st + remaining
            }
            trace("[RRQ] preparing to send data chunk : St=%d, En=%d, Block=%d\n", st, en, block)

            copy(dataout.payload[0:], payload[st:en])
            dataout.sz = en - st
        }

        encoded_dataout := Encode(dataout)
        sent_bytes, err := sessionconn.WriteToUDP(encoded_dataout.Bytes(), clientaddr) 
        chk_err(err)
        trace("[RRQ] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", dataout.String(), sent_bytes, sessionaddr.String(), clientaddr.String());

        remaining = remaining - dataout.sz
        trace("[RRQ] Remaining=%d\n", remaining)
        if remaining <= 0 {
            // if the last chunk is exactly 512 bytes, then we need to
            // send a 0 byte payload to indicate EOF
            if dataout.sz == chunk_sz {
                send_zero_eof = true
            } else {
                completed = true
                trace("[RRQ] all bytes sent out for File=%s\n", key)
            }
        }

        // wait for ack
        var buffer [1500]byte;
        n, session_src_addr, err := sessionconn.ReadFromUDP(buffer[0:])
        chk_err(err)
        trace("[RRQ] <read> : data=%s, bytes=%d, src=%s, dst=%s\n", string(buffer[0:n]), n, session_src_addr.String(), sessionaddr.String())

        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[RRQ] <message-in>:%s\n", datain.String())

        if datain.opcode == 4 {
            trace("%s\n", "[RRQ] received ACK for sent DATA")

            if completed == true {
                // were waiting for the last ACK which we recieved
                trace("[RRQ] COMPLETED : received last ack, Key=%s\n", key)
                break;
            }
            
        } else {
            trace("[RRQ] %s\n", "Invalid Request For Control Loop")
        }
    }
}

// ---------------------------------
// In-Memory File Storage ( concurrent-safe )
// ---------------------------------
type File struct {
    buf []byte
}

var filestore = struct {
    sync.RWMutex
    t map[string]*File
} { t : make(map[string]*File) }

func create_file(payload []byte) (file *File) {
    f := new(File)
    f.buf = make([]byte, len(payload))
    copy(f.buf, payload)
    return f
}

func put(key string, payload []byte) (bool) {
    file := create_file(payload)

    filestore.Lock()
    defer filestore.Unlock()

    filestore.t[key] = file
    return true
}

func get(key string) (file *File, exists bool) {
    filestore.RLock()
    defer filestore.RUnlock()

    v, ok := filestore.t[key]
    return v, ok
}

// ---------------------------------
// Test Client
// ---------------------------------
func write_file(key string, payload_sz int) (string, bool) {

    payload := generate_random_bytes(payload_sz)

    // Setup a UDP socket on which we can listen for events
    session_src_addr, err := net.ResolveUDPAddr("udp", "localhost:0")
    chk_err(err)
    session_src_conn, err := net.ListenUDP("udp", session_src_addr)
    chk_err(err)

    // send a WRQ message
    msg := new(Message)
    msg.opcode = 1
    msg.key = key
    encoded := Encode(msg)
    server_control_addr, err := net.ResolveUDPAddr("udp", control_port)
    chk_err(err)
    n, err := session_src_conn.WriteToUDP(encoded.Bytes(), server_control_addr) 
    chk_err(err)
    trace("[CLIENT] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", msg.String(), n, session_src_addr.String(), server_control_addr.String());

    // write the data
    completed := false
    send_zero_eof := false
    remaining := payload_sz;
    st := 0
    en := 0
    for {
        // wait and read the message
        var buffer [1500]byte;
        n, session_dst_addr, err := session_src_conn.ReadFromUDP(buffer[0:])
        chk_err(err)
        trace("[CLIENT] <read> : data=%s, bytes=%d, src=%s, dst=%s\n", string(buffer[0:n]), n, session_src_addr.String(), session_dst_addr.String())

        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[CLIENT] <message-in>:%s\n", datain.String())

        if datain.opcode == 4 {
            trace("%s\n", "[CLIENT] received ACK, Sending DATA")

            if completed == true {
                // were waiting for the last ACK which we recieved
                trace("%s\n", "[CLIENT] COMPLETED : received last ack, Key=%s\n", key)
                break;
            }

            dataout := new(Message)
            dataout.opcode = 3
            dataout.block = datain.block + 1

            // determine chunk to send
            if send_zero_eof == true {
                dataout.sz = 0
                var emptybuf [0]byte
                copy(dataout.payload[0:], emptybuf[0:0])
                trace("[CLIENT] preparing to send zero packet eof : Block=%d\n", dataout.block)
            } else {
                st = en
                if (st + chunk_sz) < (st + remaining) {
                    en = st + chunk_sz
                } else {
                    en = st + remaining
                }
                trace("[CLIENT] preparing to send data chunk : St=%d, En=%d, Block=%d\n", st, en, dataout.block)

                copy(dataout.payload[0:], payload[st:en])
                dataout.sz = en - st
            }

            encoded_dataout := Encode(dataout)
            sent_bytes, err := session_src_conn.WriteToUDP(encoded_dataout.Bytes(), session_dst_addr) 
            chk_err(err)
            trace("[CLIENT] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", dataout.String(), sent_bytes, session_src_addr.String(), session_dst_addr.String());

            remaining = remaining - dataout.sz
            trace("[CLIENT] Remaining=%d\n", remaining)
            if remaining <= 0 {
                // if the last chunk is exactly 512 bytes, then we need to
                // send a 0 byte payload to indicate EOF
                if dataout.sz == chunk_sz {
                    send_zero_eof = true
                } else {
                    completed = true
                    trace("[CLIENT] all bytes sent out for File=%s\n", key)
                }
            }

        } else {
            trace("[%s] %s\n", "CLIENT", "Invalid Request For Control Loop")
        }
    }

    return key, true
}

func read_file(key string) {
    // Setup a UDP socket on which we can listen for events
    session_src_addr, err := net.ResolveUDPAddr("udp", "localhost:0")
    chk_err(err)
    session_src_conn, err := net.ListenUDP("udp", session_src_addr)
    chk_err(err)

    // send a RRQ message
    msg := new(Message)
    msg.opcode = 2
    msg.key = key
    encoded := Encode(msg)
    server_control_addr, err := net.ResolveUDPAddr("udp", control_port)
    chk_err(err)
    n, err := session_src_conn.WriteToUDP(encoded.Bytes(), server_control_addr) 
    chk_err(err)
    trace("[CLIENT] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", msg.String(), n, session_src_addr.String(), server_control_addr.String());

    // receive data
    transfer_state := new(FileTransferStateIn)
    completed := false
    for {
        trace("[CLIENT] %s\n", "waiting for DATA to arrive for RRQ")

        // == recvmsg == ( IO BLOCK : wait for data packets )
        var buffer [1500]byte;
        received_bytes, serveraddr, err := session_src_conn.ReadFromUDP(buffer[0:])
        chk_err(err)
        trace("[CLIENT] <read> : data=%s, bytes=%d, src=%s\n", base64.URLEncoding.EncodeToString(buffer[0:received_bytes]), received_bytes, serveraddr.String())

        // decode the packet
        datain := Decode(bytes.NewBuffer(buffer[0:received_bytes]))
        trace("[CLIENT] <message-in>:%s\n", datain.String())

        // collect the data
        if datain.opcode == 3 {
            trace("[CLIENT] %s\n", "GOT DATA!!")

            if datain.block != transfer_state.last_block_received + 1 {
                trace("[CLIENT] Block Sequence Error, Actual=%d, Expected=%d, Message=%s\n", datain.block, transfer_state.last_block_received + 1, datain.String())

                // send ack
                er := new(Message)
                er.opcode = 5
                er.errmsg = "Invalid Block Sequence"
                encoded_er := Encode(er)
                n, err := session_src_conn.WriteToUDP(encoded_er.Bytes(), serveraddr) 
                chk_err(err)
                trace("[%s] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", er.String(), n, session_src_addr.String(), serveraddr.String());

                trace("[CLIENT] Terminating RRQ Request Session")
                break;
            }

            // append/store data in temp buffer
            transfer_state.buf.Write(datain.payload[:])
            transfer_state.last_block_received = datain.block

            // send ack
            ack := new(Message)
            ack.opcode = 4
            ack.block = datain.block
            encoded_ack := Encode(ack)
            n, err := session_src_conn.WriteToUDP(encoded_ack.Bytes(), serveraddr) 
            chk_err(err)
            trace("[CLIENT] <send> : message-out=%s, bytes=%d, src=%s, dst=%s\n", ack.String(), n, session_src_addr.String(), serveraddr.String());

            // If EOF, complete the file storage transaction
            if received_bytes < ( chunk_sz + tftp_data_header_bytes ) {
                completed = true
                break
            }
        } else {
            trace("[CLIENT] %s\n", "Invalid Request For Control Loop")
        }
    }

    if completed == true {
        // store the file
        trace("[CLIENT] data receieved fully for Key=%s\n", key)
        trace("[CLIENT] RRQ RECIEVE COMPLETED, File=%s\n", key)
    }
}

// ---------------------------------
// Test Stuff
// ---------------------------------
func generate_random_bytes(sz int) (buf []byte) {
    b := make([]byte, sz)
    _, err := rand.Read(b)
    chk_err(err)
    return b
}

// ---------------------------------
// Working/Learning Notes : 
// ---------------------------------

// proxy
//  - addr using ResolveUDPAddr -> UDPAddr
//  - bind using ListenUDP -> UDPConn
//  - recvfrom using UDPConn.ReadFromUDP -> src UDPAddr
//  - 'socket' using UPDDial -> takes src, dst -> UDPConn
//  - sendto using UDPConn.WriteToUDP(data, dst:UDPAddr)

// how to send a UDP message
// a. get the addr for server : net.ResolveUDPAddr("udp", "localhost:9991") -> UDPAddr
// b. create a socket : net.DialUDP("udp", nil (ephemeral), dstaddr:UDPAddr) -> UDPConn
// c. write data : UDPConn.writeToUDP(data);

// how to read a message
// a. get the addr for server : net.ResolveUDPAddr("udp", "localhost:9991") -> UDPAddr
// b. bind/socket : net.ListenUDP("udp", "localhost:9991") -> UDPConn
// c. wait/read the data : UDPConn.ReadFromUDP([]byte)

