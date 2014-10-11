System Description And Requirements
-----------------------------------
- A minimal TFTP server to be built which follows the protocol defined in RFC
  1350
- UDP Protocol
- Stores files in-memory
- Supports only octet/binary mode 
- Concurrent requests/sessions should be handled
- Partial byte-streams ( during upload ) should not be visible

Features Implemented ( and Not )
--------
- Minimal TFTP server which mostly implements the functionality. WRQ and RRQ
  are implemented as per RFC 1350 with some skips in error handling and robustness
- Only supports binary/octet, does not know anything about modes
- UDP Protocol
- Stores files in an in-memory map string -> byte[]. It is protected for
  concurrent access. 
- Requests can be made concurrently in a scalable way. 
    - Multiple independent read and write sessions for different or same keys can proceed in parallel and at thier own speed/rate
    - Any partial-byte-stream while being written is not visible to other readers
or writers
    - Concurrent writers to the same key are ok and one will overwrite the other
without any corruption

- Shortcomings
  - I could not get the endianess parsing correctly - could not figure out the equivalent of ntohl. There are few things which come as integers like opcode, block num etc. I have treated that byte ordering to be big-endian
  - Some features not implemented as commented in source
  - Error handling and validation not robust at places

Server Design
-------------
Since we want to support multiple readers and writers to operate in asynchronous manners possibly at different rates and high levels of concurrency, the best design is to use asynchronous event-driven i/o. We want to decouple 'workers' from 'sessions. In this way large number of UDP ports which are bound and listening and can 
be supported without the use of thread-per-session model.

The distribution of work is as follows. There is a single server control loop which wait for control requests. Once a control-request ( WRQ/RRQ ) arrives the main server loop will spin off an another 'worker' to handle the session for the duration of the session's lifetime

Go language's 'goroutines' provide an excellent way to do this where the
sequence can be coded in a routine and this goroutine will be scheduled by the
Go scheduler upon io events under the covers. The complexity of dealing with
event-driven i/o and managing your state machine is hidden way beautifully

Module Design
-------------
* Server Control Loop
  - Waits for control requests WRQ/RRQ and sheperds off goroutines to handle the session
* WRQ Session Handler
  - Starts a new WRQ session by opening a new UDP socket for the session
* RRQ Session Handler
  - Starts a new RRQ session by opening a new UDP socket for the session
* TFPT Protocol Codec
  - Decodes the protocol bytes to an app level TFTP 'message'
  - Encodes the app level TFTP 'message' to a protocol frame bytes
* File Store
  - In-Memory map to hold file abstractions of byte arrays
  - Protected concurrent r/w access
* Test Client
  - Put a file to the server
  - Get a file from the server

Choice of Language
------------------
Three languages were considered : 
* C++11 - Provides easy way to write asyc event-driven code, but there was a 
learning curve for me
* Java - Need to resort to thread-per-session design which is not scalable.
  Netty can be used, but not sure how it works
* Go - Provides goroutines which provides aync event-driven i/o with
  parallelism while coding in a 'thread-type' way. Perfect. I did not know
  anything about Go, but the learning would pay-off here

How to run
----------
* Binds/Listens on port 9991
<pre><code>
$> go run src/ttftp.go
</code></pre>

How to run tests
----------------
Runs some concurrent sessions which write a random payload and read it multiple
times and verifies the in and out hashes
<pre><code>
$> go run src/ttftp.go -test
$> go run src/ttftp.go -test 2>&1 | egrep 'TESTER'
</code></pre>

Where was time spent
--------------------
I took totally about 13-15 hours to get this done and running well.
- Spent a few hours understanding the protocol and coming up with an effective
  server design. I evaluated choices of language etc.
- I choose Go. I did not know anything about Go at first. Once I read about
  goroutines, this was the perfect thing. I spent time learning and writing my
  first Go program and it was a lot of fun. Some things are quirky and I lost
  some time trying to trivial things like binary packing/unpacking and trying 
  to understand memory allocation and maps. Getting more complicated things
  like goroutines was a breeze
- Wrote the WRQ and RRQ handlers with complementing test clients to read and
  write files. Wrote some basic tests which run a bunch of concurrent writes
  and reads the files
- Spent some time writing this documentation

Traces
------
Tracing of events is by default - cannot turn it off. 
Example of Write followed by a Read session for 513 bytes ( 2 data packets )
<pre><code>
2014/10/10 20:53:39 [CLIENT] (send) : message-out=[ (WRQ) Key=key_513 ], bytes=9, src=127.0.0.1:0, dst=127.0.0.1:9991
2014/10/10 20:53:39 [SERVER] (message-in):[ (WRQ) Key=key_513 ]
2014/10/10 20:53:39 [WRQ-HANDLER] src=127.0.0.1:56609 message-in=[ (WRQ) Key=key_513 ]
2014/10/10 20:53:39 [WRQ (56609:12289)] Starting WRQ Session
2014/10/10 20:53:39 [WRQ (56609:12289)] (send) : message-out=[ (ACK) Block=0 ], bytes=4, src=:12289, dst=127.0.0.1:56609
2014/10/10 20:53:39 [CLIENT (0:12289)] (message-in):[ (ACK) Block=0 ]
2014/10/10 20:53:39 [CLIENT (0:12289)] (send) : message-out=[ (DATA) Block=1 PayloadSz=512 ], bytes=516, src=127.0.0.1:0, dst=127.0.0.1:12289
2014/10/10 20:53:39 [WRQ (56609:12289)] (message-in):[ (DATA) Block=1 PayloadSz=512 ]
2014/10/10 20:53:39 [WRQ (56609:12289)] (send) : message-out=[ (ACK) Block=1 ], bytes=4, src=:12289, dst=127.0.0.1:56609
2014/10/10 20:53:39 [CLIENT (0:12289)] (message-in):[ (ACK) Block=1 ]
2014/10/10 20:53:39 [CLIENT (0:12289)] (send) : message-out=[ (DATA) Block=2 PayloadSz=1 ], bytes=5, src=127.0.0.1:0, dst=127.0.0.1:12289
2014/10/10 20:53:39 [CLIENT (0:12289)] all bytes sent out for File=key_513
2014/10/10 20:53:39 [WRQ (56609:12289)] (message-in):[ (DATA) Block=2 PayloadSz=1 ]
2014/10/10 20:53:39 [WRQ (56609:12289)] (send) : message-out=[ (ACK) Block=2 ], bytes=4, src=:12289, dst=127.0.0.1:56609
2014/10/10 20:53:39 [WRQ (56609:12289)] data receieved fully, storing file Key=key_513
2014/10/10 20:53:39 [FILESTORE] Request to PUT file, Key=key_513, Size=513
2014/10/10 20:53:39 [WRQ (56609:12289)] COMPLETED, File=key_513
2014/10/10 20:53:39 [CLIENT (0:12289)] (message-in):[ (ACK) Block=2 ]
2014/10/10 20:53:39 [CLIENT (%s)] COMPLETED : received last ack, Key=%s
2014/10/10 20:53:39 [CLIENT] (send) : message-out=[ (RRQ) key_513 ], bytes=9, src=127.0.0.1:0, dst=127.0.0.1:9991
2014/10/10 20:53:39 [SERVER] (message-in):[ (RRQ) key_513 ]
2014/10/10 20:53:39 [RRQ-HANDLER] src=127.0.0.1:57156 message-in=[ (RRQ) key_513 ]
2014/10/10 20:53:39 [RRQ (57156:13249)] Starting WRQ Session
2014/10/10 20:53:39 [FILESTORE] Request to GET file, Key=key_513
2014/10/10 20:53:39 [RRQ (57156:13249)] (send) : message-out=[ (DATA) Block=1 PayloadSz=512 ], bytes=516, src=:13249, dst=127.0.0.1:57156
2014/10/10 20:53:39 [CLIENT (0:13249)] (message-in):[ (DATA) Block=1 PayloadSz=512 ]
2014/10/10 20:53:39 [CLIENT (0:13249)] (send) : message-out=[ (ACK) Block=1 ], bytes=4, src=127.0.0.1:0, dst=127.0.0.1:13249
2014/10/10 20:53:39 [RRQ (57156:13249)] (message-in):[ (ACK) Block=1 ]
2014/10/10 20:53:39 [RRQ (57156:13249)] (send) : message-out=[ (DATA) Block=2 PayloadSz=1 ], bytes=5, src=:13249, dst=127.0.0.1:57156
2014/10/10 20:53:39 [RRQ (57156:13249)] all bytes sent out for File=key_513
2014/10/10 20:53:39 [CLIENT (0:13249)] (message-in):[ (DATA) Block=2 PayloadSz=1 ]
2014/10/10 20:53:39 [CLIENT (0:13249)] (send) : message-out=[ (ACK) Block=2 ], bytes=4, src=127.0.0.1:0, dst=127.0.0.1:13249
2014/10/10 20:53:39 [CLIENT (0:13249)] data receieved fully for Key=key_513, bytes=%!s(int=513)
2014/10/10 20:53:39 [CLIENT (0:13249)] RRQ RECIEVE COMPLETED, File=key_513
2014/10/10 20:53:39 [RRQ (57156:13249)] (message-in):[ (ACK) Block=2 ]
2014/10/10 20:53:39 [RRQ (57156:13249)] COMPLETED : received last ack, Key=key_513
2014/10/10 20:53:39 [TESTER] [OK] write_hash=[8ed230a960affa9e7276ef90f0d32e40760beb14], read_hash=[8ed230a960affa9e7276ef90f0d32e40760beb14]
</code></pre>

