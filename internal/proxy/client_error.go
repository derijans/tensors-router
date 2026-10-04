package proxy

import (
	"errors"
	"net"
	"net/http"
	"net/url"

	"tensors-router/internal/openai"
)

const backendUnavailableMessage = "backend unavailable"

type clientMessenger interface {
	clientMessage() string
}

func clientErrorMessage(err error) string {
	var messenger clientMessenger
	if errors.As(err, &messenger) {
		return messenger.clientMessage()
	}
	if isConnectionFailure(err) {
		return backendUnavailableMessage
	}
	return err.Error()
}

func isConnectionFailure(err error) bool {
	var urlError *url.Error
	var opError *net.OpError
	return errors.As(err, &urlError) || errors.As(err, &opError)
}

func (service *Service) writeClientError(w http.ResponseWriter, status int, errorType string, err error) {
	openai.WriteError(w, status, errorType, service.loggedClientErrorMessage(err))
}

func (service *Service) loggedClientErrorMessage(err error) string {
	message := clientErrorMessage(err)
	service.logHiddenErrorDetail(err, message)
	return message
}

func (service *Service) logHiddenErrorDetail(err error, clientMessage string) {
	if clientMessage != err.Error() {
		service.logger.Printf("client error detail hidden message=%q error=%v", clientMessage, err)
	}
}
