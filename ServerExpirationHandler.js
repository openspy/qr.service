function ServerExpirationHandler(redis_connection, amqpConnection) {
    this.redis_connection = redis_connection;
    this.amqpConnection = amqpConnection;
    this.EVENT_FIRE_INTERVAL = 1 * 1000;
    this.SERVER_HEARTBEAT_TIMEOUT = 500;

    this.EVENT_EXCHANGE = "openspy.master";
    this.EVENT_ROUTEKEY = "server.event";

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
        this.StartEventTimer();    
    }.bind(this));
}
ServerExpirationHandler.prototype.StartEventTimer = function() {
    this.EventTimer = setInterval(this.ExpirationTimerEvent.bind(this), this.EVENT_FIRE_INTERVAL);
}

ServerExpirationHandler.prototype.DeleteServer = function(server_key) {
    return new Promise(function(resolve, reject) {
        this.redis_connection.hset(server_key, "deleted", 1, function(err, res) {
            if(err) return reject(err);
            this.channel.publish(this.EVENT_EXCHANGE, this.EVENT_ROUTEKEY, Buffer.from("\\del\\" + server_key));
            resolve();
        }.bind(this));
    }.bind(this));
}
ServerExpirationHandler.prototype.DoExpirationCheck = function(server_key) {
    return new Promise(function(resolve, reject) {
        this.redis_connection.hmget(server_key, ["deleted", "last_heartbeat"], function(err, res) {
            if(err) return reject(err);
            if(res[0] == null || res[1] == null) return resolve();
            if(parseInt(res[0]) == 0) {
                var hb_time = parseInt(res[1]);
                var current_time = Math.floor(new Date().getTime() / 1000);
                var diff = current_time - hb_time;
                
                if(diff > this.SERVER_HEARTBEAT_TIMEOUT) {
                    this.DeleteServer(server_key).then(resolve, reject);
                }                
            }
        }.bind(this));
    }.bind(this)); 
}
ServerExpirationHandler.prototype.ExpirationTimerEvent = function() {
    return new Promise(function(resolve, reject) {
        var handleScanResults = null;
        var performScan = function(cursor) {
            this.redis_connection.scan(cursor, "MATCH", "*:", handleScanResults.bind(this));
        }.bind(this);
        handleScanResults = function(err, res) {
            if(err) return reject(err);
            var c = parseInt(res[0]);

            var promises = [];

            if(res[1] && res[1].length > 0) {
                for(key of res[1]) {
                    var p = this.DoExpirationCheck(key);
                    promises.push(p);
                }
            }

            Promise.all(promises, function(cursor) {
                if(cursor != 0) {
                    performScan(cursor);
                } else {
                    resolve();
                }
            }.bind(this, c));
            
            
        }.bind(this);
        performScan(0);
    }.bind(this));
};
module.exports = ServerExpirationHandler;