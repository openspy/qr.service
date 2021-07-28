const Reader = require('@maxmind/geoip2-node').Reader;
function ClientMessageForwarder(redis_connection, amqpConnection) {
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

        this.GEOIP_DB_PATH = process.env.GEOIP_DB_PATH || "/home/dev/code/PyGeoIpMap/GeoLiteCity.dat";

        Reader.open(this.GEOIP_DB_PATH).then(function(reader) {
            this.geoip_reader = reader;
        }.bind(this));

        
        
    }.bind(this));
}

ClientMessageForwarder.prototype.OnGotServerEvent = function(message) {
    if(!this.geoip_reader) return;
    var sb_forwarded_message = message.content.toString();
    var msg_split = sb_forwarded_message.split("\\");     
    if(msg_split[1] == "new") {
        var server_key = msg_split[2];
        this.redis_connection.hmget(server_key, ["wan_ip"], function(err, res) {
            if(err) {
                throw err;
            }
            if(res == null && !res.length) return;
            
            try {
                var geo_data = this.geoip_reader.city(res[0]);
                if(geo_data && geo_data.country && geo_data.country.isoCode) {
                    this.redis_connection.hset(server_key + ":custkeys", "countrycode", geo_data.country.isoCode, function(err, res) {
                        if(err) throw err;
                    });
                    
                }
            } catch {
                
            }
            
        }.bind(this));
    }
}

module.exports = ClientMessageForwarder;