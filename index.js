
var redis = require('redis');
const redisURL = process.env.REDIS_URL || "redis://127.0.0.1";
var connection = redis.createClient(redisURL);
var amqp_url = process.env.RABBITMQ_URL || "amqp://guest:guest@localhost";

var amqp = require('amqplib/callback_api');


connection.on('error', function(err) {
    console.error(err);
    process.exit(-1);
});

connection.on('close', function(err) {
    console.error(err);
    process.exit(-1);
});

var ServerExpirationHandler = require('./ServerExpirationHandler');
var ClientMessageForwarder = require('./ClientMessageForwarder');
var CountryCodeAssigner = require('./CountryCodeAssigner');
var ServerProber = require('./ServerProber');
amqp.connect(amqp_url, function(err, conn) {
    if(err) {
        console.error(err);
        process.exit(-1);
    }
    conn.on('error', (err) => {
        console.error(err);
        process.exit(-1);
    });
    conn.on('close', (err) => {
        console.error(err);
        process.exit(-1);
    });
    var ServerExpirationTimer = new ServerExpirationHandler(connection, conn);    
    var qr_message_forwarder = new ClientMessageForwarder(connection, conn);
    var country_code_assigner = new CountryCodeAssigner(connection, conn);
    var prober = new ServerProber(connection, conn);
});


//every 1 min, scan all servers for servers which haven't sent a heartbeat in 5 mins, set to deleted and fire delete event


//listen for "client.message" from SB - forward to QR
//listen for "message ack" from QR - stop sending forward