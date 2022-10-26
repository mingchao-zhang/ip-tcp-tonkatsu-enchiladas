## How we abstract different layers:
* Link layer:
Our link layer only handles sending and receiving of udp packets to real IP addresses (in our case localhost:port)
* Network Layer / Forwarding logic:
Our network layer is where all the forwarding magic 🪄 happens. To be more specific, it figures out the next hop given a destination IP address and forwards data to the appropriate node by translating VIP to real IP. Also calls the appropriate handler when we are the destination for a packet.
* Application Layer:
The applications can register their handlers and protocol numbers with the network layer. When the network layer receives a packet for our node, it will call the appropriate handler and pass the data to the application.
## How our thread model for our RIP implementation works:
The RIP handler modifies the forwarding table entries - it access the network layer data through a parameter passed into its Init function

There a few goroutines involved to make RIP work
* The RIP Handler is invoked via HandlePacket in the network layer. Handle packet is run as a separate goroutine (whenever a new packet is received)
* Another goroutine runs that periodically sends the forwarding table out to all the neighboring nodes (every 5 seconds)
* Finally, there is a goroutine for periodically deleting stale (>12 seconds old) entries from the table (every 2 seconds)

## How we process IP packets:
1. We receive a UDP packet on our port, and we send it to the network layer.
2. The network layer verifies the checksum for the packet.
3. It then determines who the final destination for the packet is
    * If we are the destination, it will forward the packet to the appropriate application handler
    * Else it figures out the next hop for the final destination (dropping packets that we don't know how to forward), decrements the TTL, recalculates the Checksum and sends the packet to the next hop using the link layer.

## Tools we used:
We built the code locally and in the course Docker container
We did not use wireshark for the project, instead we trusted our guts
(We realize that using Wireshark may have made our lives easier and we will try to use it for TCP)

To build the project run `make` in the course docker container.