/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"

	"github.com/Comcast/webpa-common/concurrent"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/device/devicehealth"
	"github.com/Comcast/webpa-common/health"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/server"
	"github.com/Comcast/webpa-common/service"
	"github.com/go-kit/kit/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	applicationName       = "talaria"
	release               = "Developer"
	defaultVnodeCount int = 211
)

// startDeviceManagement handles the configuration and initialization of the device management subsystem
// for talaria.  The returned HTTP handler can be used for device connections and messages, while the returned
// Manager can be used to route and administer the set of connected devices.
func startDeviceManagement(logger log.Logger, h *health.Health, v *viper.Viper) (http.Handler, device.Manager, error) {
	deviceOptions, err := device.NewOptions(logger, v.Sub(device.DeviceManagerKey))
	if err != nil {
		return nil, nil, err
	}

	outbounder, err := NewOutbounder(logger, v.Sub(OutbounderKey))
	if err != nil {
		return nil, nil, err
	}

	outboundListener, err := outbounder.Start()
	if err != nil {
		return nil, nil, err
	}

	deviceOptions.Listeners = []device.Listener{
		outboundListener,
		(&devicehealth.Listener{
			Dispatcher: h,
		}).OnDeviceEvent,
	}

	manager := device.NewManager(deviceOptions, nil)
	primaryHandler, err := NewPrimaryHandler(logger, manager, v)
	return primaryHandler, manager, err
}

// talaria is the driver function for Talaria.  It performs everything main() would do,
// except for obtaining the command-line arguments (which are passed to it).
func talaria(arguments []string) int {
	//
	// Initialize the server environment: command-line flags, Viper, logging, and the WebPA instance
	//

	var (
		f = pflag.NewFlagSet(applicationName, pflag.ContinueOnError)
		v = viper.New()

		logger, webPA, err = server.Initialize(applicationName, arguments, f, v)
		infoLog            = logging.Info(logger)
		errorLog           = logging.Error(logger)
	)

	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to initialize Viper environment", logging.ErrorKey(), err)
		return 1
	}

	//
	// Initialize the manager first, as if it fails we don't want to advertise this service
	//

	health := webPA.Health.NewHealth(logger, devicehealth.Options...)
	primaryHandler, manager, err := startDeviceManagement(logger, health, v)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to start device management", logging.ErrorKey(), err)
		return 1
	}

	_, talariaServer := webPA.Prepare(logger, health, primaryHandler)
	waitGroup, shutdown, err := concurrent.Execute(talariaServer)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to start device manager", logging.ErrorKey(), err)
		return 1
	}

	//
	// Now, initialize the service discovery infrastructure
	//

	serviceOptions, err := service.FromViper(service.Sub(v))
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to read service discovery options", logging.ErrorKey(), err)
		return 2
	}

	serviceOptions.Logger = logger
	services, err := service.New(serviceOptions)
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to initialize service discovery", logging.ErrorKey(), err)
		return 2
	}

	instancer, err := services.NewInstancer()
	if err != nil {
		errorLog.Log(logging.MessageKey(), "Unable to obtain service discovery instancer", logging.ErrorKey(), err)
		return 2
	}

	defer services.Deregister()
	services.Register()
	infoLog.Log("configurationFile", v.ConfigFileUsed(), "serviceOptions", serviceOptions)

	var (
		subscription = service.Subscribe(serviceOptions, instancer)
		signals      = make(chan os.Signal, 1)
	)

	go func() {
		first := true
		for {
			select {
			case u := <-subscription.Updates():
				// throw away the first Accessor, as that is just the initial set of talarias
				if first {
					first = false
					infoLog.Log(logging.MessageKey(), "discarding initial service discovery event")
					continue
				}

				infoLog.Log(logging.MessageKey(), "new talaria instances")
				manager.DisconnectIf(func(candidate device.ID) bool {
					instance, err := u.Get(candidate.Bytes())
					if err != nil {
						errorLog.Log(logging.MessageKey(), "Error while attempting to rehash device", "deviceID", candidate, logging.ErrorKey(), err)
						return true
					}

					if instance != serviceOptions.Registration {
						infoLog.Log(logging.MessageKey(), "service discovery rehash", "deviceID", candidate)
						return true
					}

					return false
				})

			case <-subscription.Stopped():
				return
			}
		}
	}()

	signal.Notify(signals)
	<-signals
	close(shutdown)
	waitGroup.Wait()

	return 0
}

func main() {
	os.Exit(talaria(os.Args))
}
