package proxy

func (service *Service) onRuntimeChanged() {
	service.webUI.invalidate()
}
