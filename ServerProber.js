var dgram = require('dgram');
function ServerProber(redis_connection, amqpConnection) {
    this.EVENT_EXCHANGE = "openspy.master";
    this.EVENT_SERVER_EVENT_ROUTEKEY = "server.event";

    this.amqpConnection = amqpConnection;
    this.redis_connection = redis_connection;

    this.amqpConnection.createChannel(function(err, ch) {
        this.channel = ch;
        ch.assertExchange(this.EVENT_EXCHANGE, 'topic', {durable: true});

        ch.assertQueue('', {exclusive: true}, function(err, q) {
            ch.bindQueue(q.queue, this.EVENT_EXCHANGE, this.EVENT_SERVER_EVENT_ROUTEKEY);

            ch.consume(q.queue, this.OnGotServerEvent.bind(this), {noAck: true});
        }.bind(this));  
    }.bind(this));

    this.probe_socket = dgram.createSocket('udp4');
    this.probe_socket.on('message', this.onProbeSocketGotData.bind(this));
}

ServerProber.prototype.OnGotServerEvent = function(message) {
    var sb_forwarded_message = message.content.toString();
    var msg_split = sb_forwarded_message.split("\\");     
    if(msg_split[1] == "new") {
        var server_key = msg_split[2];
        this.redis_connection.hmget(server_key, ["wan_ip", "wan_port", "instance_key"], function(err, res) {
            if(err) {
                throw err;
            }
            if(res == null && !res.length) return;
            if(res[2] == null) return; //only scan v2 servers
            this.ProbeServerIPPort(res[0], parseInt(res[1]));
            
        }.bind(this));
    }
}
ServerProber.prototype.ProbeServerIPPort = function(ip_address, port) {
    var v2_buffer = Buffer.from("fefd0000000000ff000000", "hex"); //v2 buffer - no prequery ip verify
    this.probe_socket.send(v2_buffer, port, ip_address);
}

ServerProber.prototype.onProbeSocketGotData = function(msg, rinfo) {
    var key = "IPMAP_" + rinfo.address + "-" + rinfo.port;
    this.redis_connection.get(key, function(address, err, server_key) {
        if(err) throw err;
        if(server_key) {
            this.redis_connection.hmset(server_key, "allow_unsolicited_udp", 1, "icmp_address", address, function(err, res) {
                if(err) throw err;
            }.bind(this));
        }
    }.bind(this, rinfo.address));
}
module.exports = ServerProber;