package main

import(
    "fmt"
    "net"
    "os"
//    "time"
    "bytes"
    "encoding/binary"
    "log"
    "crypto/rand"
)

// ---------------------------------
// Utilities
// ---------------------------------
func check_error(err error) {
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
        os.Exit(1)
    }
}

func trace(format string, v ...interface{}) {
    log.Printf(format, v...)
}

// ---------------------------------
// TFTP Protocol Encoding/Decoding
// ---------------------------------
type Message struct {
    opcode uint16
    key string
    payload [512]byte
    block uint16
    errcode uint16
    errmsg string
    sz int
}

// func (m Message) String() (string) {
//     s := "{ "
//     s += "<" + m.opcode + ">"
//     if m.opcode == 1 {
//         s += " " + m.key
//     } else if m.opcode == 2 {
//         s += " " + m.key
//     } else if m.opcode == 3 {
//         s += " " + m.block
//         s += " " + m.sz
//         s += " " + m.payload
//     } else if m.opcode == 4 {
//         s += " " + m.block
//     } else {
//     }
// }
    
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
        binary.Read(buf, binary.BigEndian, &m.block);
        sz, _ := buf.Read(m.payload[0:512])
        m.sz = sz
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
        buf.Write([]byte(m.key))
    } else if opcode == 2 {
        buf.Write([]byte(m.key))
    } else if opcode == 3 {
        binary.Write(buf, binary.BigEndian, uint16(m.block))
        buf.Write([]byte(m.payload[0:m.sz]))
    } else if opcode == 4 {
        binary.Write(buf, binary.BigEndian, uint16(m.block))
    } else if opcode == 5 {
        buf.Write([]byte("NODATA"))
    } else {
        buf.Write([]byte("NODATA"))
    }

    return buf
}

func CodecTest() {
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
    fmt.Println(decoded)
}

// ---------------------------------
// TFTP Control Service
// ---------------------------------
func main() {
    // Control Server UDP Socket
    serveraddr, err := net.ResolveUDPAddr("udp", ":9991")
    check_error(err)
    serverconn, err := net.ListenUDP("udp", serveraddr)
    check_error(err)

    // TESTING MESSAGES
    // go routine to create and send a message to this server
    // go send_message()
    go test_wrq_client(31412)

    // Control Loop
    for {
        // == recvmsg ==
        var buffer [1500]byte;
        n, clientaddr, err := serverconn.ReadFromUDP(buffer[0:])
        check_error(err)
        trace("[%s] <read> : data=%s, bytes=%d, src=%s\n", string(buffer[0:n]), "SERVER", n, clientaddr.String())

        // decode the message
        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[%s] <message-in>:[ %s ]\n", "SERVER", datain)

        // orchestrate
        if datain.opcode == 1 {
            go wrq_session(datain, clientaddr)
        } else if datain.opcode == 2 {
            go rrq_session(datain, clientaddr)
        } else {
            trace("[%s] %s\n", "SERVER", "Invalid Request For Control Loop")
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
    trace("[%s] src=%s message=%s\n", "WRQ Handler", clientaddr.String(), m)

    // 1. bind a new udp socket ( ListenUDP ) this is our new 'endpoint' for the session
    //var tid uint16
    //tid = 9999
    sessionaddr, err := net.ResolveUDPAddr("udp", ":9999")
    check_error(err)
    sessionconn, err := net.ListenUDP("udp", sessionaddr)
    check_error(err)

    // 2. send the initial ACK for WRQ transfer initiate
    first_ack := new(Message)
    first_ack.opcode = 4
    first_ack.block = 0
    encoded_first_ack := Encode(first_ack)
    n, err := sessionconn.WriteToUDP(encoded_first_ack.Bytes(), clientaddr) 
    check_error(err)
    trace("[%s] <send> : data=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", first_ack, n, sessionaddr.String(), clientaddr.String());

    // 3. Read DATA Blocks
    transfer_state := new(FileTransferStateIn)
    completed := false
    for {
        trace("%s\n", "Waiting on WRQ session loop")

        // == recvmsg == ( IO BLOCK : wait for data packets )
        var buffer [1500]byte;
        received_bytes, clientaddr, err := sessionconn.ReadFromUDP(buffer[0:])
        check_error(err)
        trace("[%s] <read> : data=%s, bytes=%d, src=%s\n", "WRQ", string(buffer[0:n]), received_bytes, clientaddr.String())

        // [TODO] validate clientaddr to ensure no cross-talk among sessions ( ignoring for now )

        // decode the packet
        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[%s] <message-in>:[ %d %d %d %s ]\n", "WRQ", datain.opcode, datain.block, datain.sz, datain.payload)

        // collect the data
        if datain.opcode == 3 {
            trace("%s\n", "GOT DATA!!")

            if datain.block != transfer_state.last_block_received + 1 {
                trace("Block Sequence Error, Actual=%d, Expected=%d", datain.block, transfer_state.last_block_received + 1)

                // send ack
                er := new(Message)
                er.opcode = 5
                er.errmsg = "Invalid Block Sequence"
                encoded_er := Encode(er)
                n, err := sessionconn.WriteToUDP(encoded_er.Bytes(), clientaddr) 
                check_error(err)
                trace("[%s] <send> : data=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", er, n, sessionaddr.String(), clientaddr.String());

                trace("Terminating WRQ Session")
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
            check_error(err)
            trace("[%s] <send> : data=%s, bytes=%d, src=%s, dst=%s\n", "WRQ", ack, n, sessionaddr.String(), clientaddr.String());

            // If EOF, complete the file storage transaction
            if received_bytes < 516 {
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

func test_wrq_client(payload_sz int) {
    // Setup a UDP socket on which we can listen for events
    session_src_addr, err := net.ResolveUDPAddr("udp", "localhost:0")
    check_error(err)
    session_src_conn, err := net.ListenUDP("udp", session_src_addr)
    check_error(err)

    // send a wrq message
    msg := new(Message)
    msg.opcode = 1
    msg.key = "key-1"
    encoded := Encode(msg)
    server_control_addr, err := net.ResolveUDPAddr("udp", "localhost:9991")
    check_error(err)
    n, err := session_src_conn.WriteToUDP(encoded.Bytes(), server_control_addr) 
    check_error(err)
    trace("[CLIENT] <send> : data=%s, bytes=%d, src=%s, dst=%s\n", msg, n, session_src_addr.String(), server_control_addr.String());

    chunk_sz := 512

    payload := generate_random_bytes(payload_sz)
    remaining := payload_sz;
    st := -1
    en := -1
    // wait for ack
    for {
        // wait and read the message
        var buffer [1500]byte;
        n, session_dst_addr, err := session_src_conn.ReadFromUDP(buffer[0:])
        check_error(err)

        trace("[CLIENT] <read> : data=%s, bytes=%d, src=%s, dst=%s\n", string(buffer[0:n]), n, session_src_addr.String(), session_dst_addr.String())
        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        trace("[CLIENT] <message-in>:[ %s ]\n", datain)

        if datain.opcode == 4 {
            trace("%s\n", "[CLIENT] received ACK, Sending DATA")

            st = en + 1
            if (st + chunk_sz - 1) < (st + remaining - 1) {
                en = st + chunk_sz - 1
            } else {
                en = st + remaining - 1
            }

            trace("St=%d, En=%d\n", st, en)

            dataout := new(Message)
            dataout.opcode = 3
            dataout.block = datain.block + 1
            copy(dataout.payload[:], payload[st:en])
            dataout.sz = en - st + 1
            encoded_dataout := Encode(dataout)
        
            sent_bytes, err := session_src_conn.WriteToUDP(encoded_dataout.Bytes(), session_dst_addr) 
            check_error(err)
            trace("[CLIENT] <send> : data=%s, bytes=%d, src=%s, dst=%s\n", msg, sent_bytes, session_src_addr.String(), session_dst_addr.String());

            remaining = remaining - dataout.sz
            trace("Remaining=%d\n", remaining)
            if remaining <= 0 {
                trace("[CLIENT] COMPLETE sending of File=%s\n", msg.key)
                break
            }

        } else {
            trace("[%s] %s\n", "CLIENT", "Invalid Request For Control Loop")
        }
    }

    // send a data message
}

// ---------------------------------
// RRQ Session Handler
// ---------------------------------
func rrq_session(m *Message, clientaddr *net.UDPAddr) {
    trace("[%s] %d %s %s\n", "RRQ Handler", m.opcode, m.key, clientaddr.String())

    // generate a tid
    // bind a new udp socket ( ListenUDP ) this is our new connection

    // validate if file is present else respond with error

    // while there is more data to be sent
    //  send the data packet
    //  wait for ack
    //  once ack'd continue till EOF
}

// ---------------------------------
// File Storage
// ---------------------------------

// ---------------------------------
// Test Client
// ---------------------------------
func read_file() {
}

func write_file() {
}

// ---------------------------------
// Test Stuff
// ---------------------------------
func generate_random_bytes(sz int) (buf []byte) {
    b := make([]byte, sz)
    _, err := rand.Read(buf)
    check_error(err)
    return b
}

func send_message() {
    dstaddr, err := net.ResolveUDPAddr("udp", "localhost:9991")
    check_error(err)

    dstconn, err := net.DialUDP("udp", nil, dstaddr)
    check_error(err)

    // msg := "<HOLA>:<" + time.Now().String() + ">"
    // fmt.Println(00, msg)
    // n, err := dstconn.Write([]byte(msg)) 

    msg := new(Message)
    msg.opcode = 3
    msg.block = 213
    payload :=  "asdfaksdjflkasjdfjaslkdfjlaksdaadsfa"
    copy(msg.payload[:], payload)
    msg.sz = len(payload)
    encoded := Encode(msg)
    // fmt.Println(encoded.Bytes())
    n, err := dstconn.Write(encoded.Bytes()) 
    check_error(err)
    trace("[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());
    
    msg = new(Message)
    msg.opcode = 2
    msg.key = "key-1"
    encoded = Encode(msg)
    n, err = dstconn.Write(encoded.Bytes()) 
    check_error(err)
    trace("[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());

    msg = new(Message)
    msg.opcode = 1
    msg.key = "key-1"
    encoded = Encode(msg)
    n, err = dstconn.Write(encoded.Bytes()) 
    check_error(err)
    trace("[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());
}

func read_message() {
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

