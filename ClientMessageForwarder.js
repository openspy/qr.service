function ClientMessageForwarder(redis_connection, amqpConnection) {
    this.EVENT_EXCHANGE = "openspy.master";
    this.EVENT_ROUTEKEY = "qr.message";
    this.EVENT_CLIENT_MESSAGE_ACK_ROUTEKEY = "client-messages.acks";
    this.EVENT_SB_MESSAGE_ROUTEKEY = "client.message";

    this.amqpConnection = amqpConnection;
    this.redis_connection = redis_connection;

    this.client_message_resend_timers = {};
    this.CLIENT_MESSAGE_RESNED_INTERVAL = 1000;
    this.CLIENT_MESSAGE_MAX_RESENDS = 5;

    this.amqpConnection.createChannel(function(err, ch) {
        if(err) {
            console.error(err);
            process.exit(-1);
        }
        ch.on('error', (err) => {
            console.error(err);
            process.exit(-1);
        });
        this.channel = ch;
        ch.assertExchange(this.EVENT_EXCHANGE, 'topic', {durable: true});
        ch.assertQueue('', {exclusive: true}, function(err, q) {
            ch.bindQueue(q.queue, this.EVENT_EXCHANGE, this.EVENT_CLIENT_MESSAGE_ACK_ROUTEKEY);

            ch.consume(q.queue, this.OnClientMessageAck.bind(this), {noAck: true});
        }.bind(this));

        ch.assertQueue('', {exclusive: true}, function(err, q) {
            ch.bindQueue(q.queue, this.EVENT_EXCHANGE, this.EVENT_SB_MESSAGE_ROUTEKEY);

            ch.consume(q.queue, this.OnGotClientMessageFromServerBrowsing.bind(this), {noAck: true});
        }.bind(this));  
        
    }.bind(this));
}

ClientMessageForwarder.prototype.GetClientMessageUniqueKey = function(driver_hostname, driver_address, instance_key, to_address, key) {
    return driver_hostname + ":" + driver_address + ":" + to_address + ":" + instance_key + ":" + key;
}
ClientMessageForwarder.prototype.OnGotClientMessageFromServerBrowsing = function(message) {
    var sb_forwarded_message = message.content.toString();
    var msg_split = sb_forwarded_message.split("\\");        

    var to_ip = msg_split[5]
    var to_port = msg_split[6];
    var message = msg_split[7];
    var ip_string = to_ip + ":" + to_port;
    this.ForwardMessageToQR(ip_string, message);   
}
ClientMessageForwarder.prototype.OnClientMessageAck = function(message) {
    var msg = JSON.parse(message.content.toString());
    var key = this.GetClientMessageUniqueKey(msg.hostname, msg.driver_address, msg.instance_key, msg.from_address, msg.identifier);
    if(this.client_message_resend_timers[key] !== undefined) {
        clearInterval(this.client_message_resend_timers[key].timer);
        delete this.client_message_resend_timers[key];
    }
}

ClientMessageForwarder.prototype.ForwardMessageToQR = function(to_address, b64_string) {
    var key = "IPMAP_" + to_address.replace(":", "-");
    this.redis_connection.get(key,  function(err, res) {
        if(err) throw err;
        if(res != null) {
            this.redis_connection.hmget(res, ["instance_key", "driver_hostname", "driver_address"], function(err, res) {

                if(!res || !res.length || !res[0]) return;

                //generate unique key, and add to resend queue
                //forward to QR every 1 second, up to 5 times - or until unique key ack is received
                var identifier = Math.floor(Math.random() * ((Math.pow(2,32)/2)-1));
                var event_message = {hostname: res[1], driver_address: res[2], to_address, message: b64_string, version: 2, instance_key: parseInt(res[0]), identifier, type: "client_message"};
                var interval_key = this.GetClientMessageUniqueKey(event_message.hostname, event_message.driver_address, event_message.instance_key, event_message.to_address, event_message.identifier);
                if(this.client_message_resend_timers[interval_key] === undefined) {
                    this.client_message_resend_timers[interval_key] = {resends: 0};
                } else {
                    return; //XXX: this should not happen, throw error
                }
                this.client_message_resend_timers[interval_key].timer = setInterval(function(resend_data, interval_key, event_message) {
                    if(resend_data.resends > this.CLIENT_MESSAGE_MAX_RESENDS) {
                        clearInterval(resend_data.timer);
                        delete this.client_message_resend_timers[interval_key];
                        return;
                    }
                    resend_data.resends++;                    
                    var msg = Buffer.from(JSON.stringify(event_message));
                    this.channel.publish(this.EVENT_EXCHANGE, this.EVENT_ROUTEKEY, msg);
                }.bind(this, this.client_message_resend_timers[interval_key], interval_key, event_message), this.CLIENT_MESSAGE_RESNED_INTERVAL)
            }.bind(this));
        }

    }.bind(this));


}
module.exports = ClientMessageForwarder;