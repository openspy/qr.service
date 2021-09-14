var dgram = require('dgram');
const { lookup } = require('dns');
const { parse } = require('path');
function ServerProber(redis_connection, amqpConnection) {
    this.EVENT_EXCHANGE = "openspy.master";
    this.EVENT_SERVER_EVENT_ROUTEKEY = "server.event";
    this.MAX_PROBES = 5;

    this.amqpConnection = amqpConnection;
    this.redis_connection = redis_connection;

    this.GAMEID_DATABASE = 2;
    this.SERVER_DATABASE = 0;
    this.PREQUERY_IP_VERIFY_FLAG = 128;

    this.amqpConnection.createChannel(function(err, ch) {
        this.channel = ch;
        ch.assertExchange(this.EVENT_EXCHANGE, 'topic', {durable: true});

        ch.assertQueue('', {exclusive: true}, function(err, q) {
            ch.bindQueue(q.queue, this.EVENT_EXCHANGE, this.EVENT_SERVER_EVENT_ROUTEKEY);

            ch.consume(q.queue, this.OnGotServerEvent.bind(this), {noAck: true});
        }.bind(this));  
    }.bind(this));

    this.probe_socket = dgram.createSocket('udp4');
    this.probe_socket.bind(process.env.MASTER_PROBE_PORT);
    this.probe_socket.on('message', this.onProbeSocketGotData.bind(this));
}
ServerProber.prototype.CheckGameHasPrequeryIpVerify = function(gameid) {
    return new Promise(function(resolve, reject) {
        var lookup_key = "gameid_" + gameid;
        this.redis_connection.select(this.GAMEID_DATABASE, function(err) {
            if(err) {
                return reject(err);
            }
            this.redis_connection.get(lookup_key, function(err, res) {
                if(err) {
                    return reject(err);
                }
                if(res == null) return resolve(0);
                this.redis_connection.hmget(res, ["backendflags"], function(err, res) {
                    if(err) {
                        return reject(err);
                    }
                    if(res == null || res[0] == null) return resolve(0);
                    var flags = parseInt(res[0]);
                    if(flags & this.PREQUERY_IP_VERIFY_FLAG) {
                        resolve(1);
                    } else {
                        resolve(0);
                    }
                }.bind(this));
            }.bind(this));
        }.bind(this));
    }.bind(this));

}
ServerProber.prototype.deleteProbeData = function(server_key) {
    return new Promise(function(resolve, reject) {
        this.redis_connection.select(this.SERVER_DATABASE, function(err) {
            this.redis_connection.hdel(server_key, ["num_probes", "allow_unsolicited_udp", "icmp_address"], function(err, res) {
                if(err) return reject(err);
                return resolve();
            }.bind(this));
        }.bind(this));
    }.bind(this));
}
ServerProber.prototype.OnGotServerEvent = function(message) {
    var sb_forwarded_message = message.content.toString();
    var msg_split = sb_forwarded_message.split("\\");     
    
    if(msg_split[1] == "new" || msg_split[1] == "update") {
        var server_key = msg_split[2];
        if(server_key.startsWith('flatout2pc')) return; //skip flatout2 probing

        var eventHandler = function() {
            this.redis_connection.select(this.SERVER_DATABASE, function(err) {
                if(err) {
                    throw err;
                }
                this.redis_connection.hmget(server_key, ["wan_ip", "wan_port", "instance_key", "num_probes", "allow_unsolicited_udp", "gameid"], function(err, res) {
                    if(err) {
                        throw err;
                    }
                    if(res == null && !res.length) return;
                    if(res[2] == null || parseInt(res[2]) == 0) return; //only scan v2 servers
        
                    if(res[4] != null && !isNaN(parseInt(res[4]))) { //allow_unsolicited_udp is set, do not probe
                        return;
                    }
        
                    var num_probes = parseInt(res[3]) || 0;
                    if(num_probes >= this.MAX_PROBES) {
                        this.redis_connection.hmset(server_key, "allow_unsolicited_udp", 0, function(err, res) {
                            if(err) throw err;
                        }.bind(this));
                        return;
                    } else {
                        this.redis_connection.hincrby(server_key, "num_probes", 1, async function(ip, port, gameid, err, res) {
                            if(err) throw err;
                            var is_prequery_ip_verify = await this.CheckGameHasPrequeryIpVerify(gameid);
                            this.ProbeServerIPPort(ip, port, is_prequery_ip_verify);
                        }.bind(this, res[0], parseInt(res[1]), parseInt(res[5])));
                    }
                    
                    
                }.bind(this));
            }.bind(this));
        }.bind(this);

        if(msg_split[1] == "new") {
            this.deleteProbeData(server_key).then(eventHandler, function(err) {
                throw err;
            });
        } else {
            eventHandler();
        }
    }
}
ServerProber.prototype.ProbeServerIPPort = function(ip_address, port, prequery_ip_verify) {
    var v2_buffer = Buffer.from("fefd0000000000ff000000", "hex"); //v2 buffer - no prequery ip verify
    if(prequery_ip_verify) {
        v2_buffer = Buffer.from("fefd0900000000", "hex");
    }
    this.probe_socket.send(v2_buffer, port, ip_address);
}

ServerProber.prototype.onProbeSocketGotData = function(msg, rinfo) {
    var key = "IPMAP_" + rinfo.address + "-" + rinfo.port;
    this.redis_connection.select(this.SERVER_DATABASE, function(err) {    
        this.redis_connection.get(key, function(address, err, server_key) {
            if(err) throw err;
            if(server_key) {
                this.redis_connection.hmset(server_key, "allow_unsolicited_udp", 1, "icmp_address", address, function(err, res) {
                    if(err) throw err;
                }.bind(this));
            }
        }.bind(this, rinfo.address));
    }.bind(this));
}
module.exports = ServerProber;