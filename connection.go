package apns

import (
	"crypto/tls"
    "fmt"
	"log"
	"time"
    socket "appengine/socket"
    "appengine"
)

const (
	PRODUCTION_GATEWAY = "gateway.push.apple.com"
	SANDBOX_GATEWAY    = "gateway.sandbox.push.apple.com"
)

const (
    GATEWAY_PORT = 2195
)

var (
	productionConnection *gatewayConnection
	productionConfig     *tls.Config
	sandboxConnection    *gatewayConnection
	sandboxConfig        *tls.Config
)

type gatewayConnection struct {
	client  *tls.Conn
	config  *tls.Config
	gateway string
    context appengine.Context
}

// TODO: Wrap log.Print calls so we can easily disable them.

func LoadCertificate(production bool, certContents []byte, context appengine.Context) error {
	keyPair, err := tls.X509KeyPair(certContents, certContents)
	if err != nil {
		return err
	}

	if err := storeAndConnect(production, keyPair, context); err != nil {
		return err
	}

	return nil
}

func setAppengineContext(context appengine.Context){
    if(sandboxConnection != nil){
        sandboxConnection.context = context
        log.Printf("[APNS] set appengine context for sandbox")
    }

    if(productionConnection != nil){
        productionConnection.context = context
        log.Printf("[APNS] set appengine context for production")
    }
}

func LoadCertificateFile(production bool, certLocation string, context appengine.Context) error {
    log.Printf("[APNS] loading certificate")

    keyPair, err := tls.LoadX509KeyPair(certLocation, certLocation)
	if err != nil {
        log.Printf("[APNS] loading certificate failed %v", err.Error())
        return err
	}

    if err := storeAndConnect(production, keyPair, context); err != nil {
		return err
	}

	return nil
}

func storeAndConnect(production bool, keyPair tls.Certificate, context appengine.Context) error {
	if production {
		// Production Connections
		productionConfig = &tls.Config{
			Certificates: []tls.Certificate{keyPair},
			ServerName: PRODUCTION_GATEWAY,
		}

        log.Printf("[APNS] production config set")


        productionConnection = &gatewayConnection{
			gateway: fmt.Sprintf("%v:%v",PRODUCTION_GATEWAY, GATEWAY_PORT),
			config:  productionConfig,
            context: context,
		}

        log.Printf("[APNS] production connection established %v", productionConnection)


        if err := productionConnection.connect(); err != nil {
			return err
		}
	} else {
		// Sandbox Connections
		sandboxConfig = &tls.Config{
			Certificates: []tls.Certificate{keyPair},
			ServerName: SANDBOX_GATEWAY,
		}

        log.Printf("[APNS] sandbox config set")


        sandboxConnection = &gatewayConnection{
			gateway: fmt.Sprintf("%v:%v",SANDBOX_GATEWAY, GATEWAY_PORT),
			config:  sandboxConfig,
            context: context,
		}

        log.Printf("[APNS] sandbox connection established %v", sandboxConnection)

		if err := sandboxConnection.connect(); err != nil {
			return err
		}
	}

	return nil
}

func (this *gatewayConnection) connect() error {
    if(this.context != nil){
        conn, err := socket.Dial(this.context, "tcp", this.gateway)
        if err != nil {
            log.Printf("[APNS] could not open connection")
            return err
        }

        log.Printf("[APNS] connection established")

        this.client = tls.Client(conn, this.config)
        err = this.client.Handshake()
        if err != nil {
            return err
        }
        log.Printf("[APNS] New client")

    } else{
        log.Printf("[APNS] no appengine.Context set")
    }

	return nil
}

func (this *gatewayConnection) Write(payload []byte) error {
    if this != nil {
        if(this.client != nil){
            _, err := this.client.Write(payload)
            if err != nil {
                // We probably disconnected. Reconnect and resend the message.
                // TODO: Might want to check the actual error returned?
                log.Printf("[APNS] Error writing data to socket: %v", err)
                log.Println("[APNS] *** Server disconnected unexpectedly. ***")
                err := this.connect()
                if err != nil {
                log.Printf("[APNS] Could not reconnect to the server: %v", err)
                return err
                }
                log.Println("[APNS] Reconnected to the server successfully.")

                // TODO: This could cause an endless loop of errors.
                // 		If it's the connection failing, that would be caught above.
                // 		So why don't we add a counter to the payload itself?
                this.Write(payload)
                defer this.client.Close()
            }
        } else{
            log.Printf("[APNS no client configured]")
        }
    } else {
        log.Printf("[APNS] no gateway configured")
    }

	return nil
}

func (this *gatewayConnection) ReadErrors() (bool, []byte) {
	_ = this.client.SetReadDeadline(time.Now().Add(5 * time.Second))

	buffer := make([]byte, 6, 6)
	n, _ := this.client.Read(buffer)

	// n == 0 if there were no errors.
	if n == 0 {
		// TODO: I think this would get returned even if Read() produces an error.
		return false, nil
	}

	return true, buffer
}
