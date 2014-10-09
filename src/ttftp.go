package main

import(
    "fmt"
    "net"
    "os"
    "time"
    "bytes"
    "encoding/binary"
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

// ---------------------------------
// Communication
// ---------------------------------
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
    fmt.Fprintf(os.Stdout, "[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());
    
    msg = new(Message)
    msg.opcode = 2
    msg.key = "key-1"
    encoded = Encode(msg)
    n, err = dstconn.Write(encoded.Bytes()) 
    check_error(err)
    fmt.Fprintf(os.Stdout, "[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());

    msg = new(Message)
    msg.opcode = 1
    msg.key = "key-1"
    encoded = Encode(msg)
    n, err = dstconn.Write(encoded.Bytes()) 
    check_error(err)
    fmt.Fprintf(os.Stdout, "[CLIENT] <send> : data=%s, bytes=%d, dst=%s\n", msg, n, dstaddr.String());
}

func read_message() {
}

// ---------------------------------
// Protocol Data Encoding/Decoding
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
        errmsg, _ := buf.ReadString(byte(0));
        m.errmsg = errmsg
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
        fmt.Println("HOLA")
        binary.Write(buf, binary.BigEndian, uint16(m.block))
        buf.Write([]byte(m.payload[0:m.sz]))
    } else if opcode == 4 {
        buf.Write([]byte("NODATA"))
    } else if opcode == 5 {
        buf.Write([]byte("NODATA"))
    } else {
        buf.Write([]byte("NODATA"))
    }

    return buf
}

func CodecTest() {

    // testing
    msg := new(Message)
    msg.opcode = 3
    msg.block = 213
    payload :=  "asdfaksdjflkasjdfjaslkdfjlaksdaadsfa"
    copy(msg.payload[:], payload)
    msg.sz = len(payload)
    encoded := Encode(msg)
    fmt.Println(encoded.Bytes())

    // testing
    decoded := Decode(encoded)
    fmt.Println(decoded)
}

// ---------------------------------
// TFTP Control Service
// ---------------------------------
func main() {
    serveraddr, err := net.ResolveUDPAddr("udp", ":9991")
    check_error(err)

    serverconn, err := net.ListenUDP("udp", serveraddr)
    check_error(err)

    // go routine to create and send a message to this server
    go send_message()

    for {
        // wait and read the message
        var buffer [1500]byte;
        n, clientaddr, err := serverconn.ReadFromUDP(buffer[0:])
        check_error(err)

        fmt.Fprintf(os.Stdout, "[SERVER] <read> : data=%s, bytes=%d, src=%s\n", string(buffer[0:n]), n, clientaddr.String())
        datain := Decode(bytes.NewBuffer(buffer[0:n]))
        fmt.Fprintf(os.Stdout, "[SERVER] <message-in>:[ %d %s %d %d %s ]\n", datain.opcode, datain.key, datain.block, datain.sz, datain.payload)

        if datain.opcode == 1 {
            go wrq_session(datain, clientaddr)
        } else if datain.opcode == 2 {
            go rrq_session(datain, clientaddr)
        } else {
            fmt.Fprintf(os.Stdout, "[%s] %s\n", "SERVER", "Invalid Request For Control Loop")
        }
    }

    time.Sleep(time.Second * 30);
}

// ---------------------------------
// WRQ Service
// ---------------------------------
type FileTransferStateIn struct {
    buf bytes.Buffer
    last_block_received uint16
}

func wrq_session(m *Message, clientaddr *net.UDPAddr) {
    fmt.Fprintf(os.Stdout, "[%s] %d %s %s\n", "WRQ Handler", m.opcode, m.key, clientaddr.String())

    // generate a tid
    // bind a new udp socket ( ListenUDP ) this is our new connection
    // send the initial packet for WRQ transfer initiate

    // while more data to be received
    //  recvfrom - wait
    //  read the data and encode
    //  validate and respond
    //  append data to buf chain
    //  if full message recvd, update in file storage and done
}

// ---------------------------------
// RRQ Service
// ---------------------------------
func rrq_session(m *Message, clientaddr *net.UDPAddr) {
    fmt.Fprintf(os.Stdout, "[%s] %d %s %s\n", "RRQ Handler", m.opcode, m.key, clientaddr.String())

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

