package steps

import (
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/executor/depot/log_streamer"
	"code.cloudfoundry.org/lager"
	"github.com/tedsuo/ifrit"
)

const (
	timeoutMessage          = "Timed out after %s: health check never passed.\n"
	timeoutCrashReason      = "Instance never healthy after %s: %s"
	healthcheckNowUnhealthy = "Instance became unhealthy: %s"
)

type healthCheckStep struct {
	readinessCheck ifrit.Runner
	livenessCheck  ifrit.Runner

	logger              lager.Logger
	clock               clock.Clock
	logStreamer         log_streamer.LogStreamer
	healthCheckStreamer log_streamer.LogStreamer

	startTimeout time.Duration

	*canceller
}

func NewHealthCheckStep(
	readinessCheck ifrit.Runner,
	livenessCheck ifrit.Runner,
	logger lager.Logger,
	clock clock.Clock,
	logStreamer log_streamer.LogStreamer,
	healthcheckStreamer log_streamer.LogStreamer,
	startTimeout time.Duration,
) ifrit.Runner {
	logger = logger.Session("health-check-step")

	return &healthCheckStep{
		readinessCheck:      readinessCheck,
		livenessCheck:       livenessCheck,
		logger:              logger,
		clock:               clock,
		logStreamer:         logStreamer,
		healthCheckStreamer: healthcheckStreamer,
		startTimeout:        startTimeout,
		canceller:           newCanceller(),
	}
}

func (step *healthCheckStep) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	fmt.Fprint(step.logStreamer.Stdout(), "Starting health monitoring of container\n")

	readinessProcess := ifrit.Background(step.readinessCheck)

	select {
	case err := <-readinessProcess.Wait():
		if err != nil {
			fmt.Fprintf(step.healthCheckStreamer.Stderr(), "%s\n", err.Error())
			fmt.Fprintf(step.logStreamer.Stderr(), timeoutMessage, step.startTimeout)
			step.logger.Info("timed-out-before-healthy", lager.Data{
				"step-error": err.Error(),
			})
			return NewEmittableError(err, timeoutCrashReason, step.startTimeout, err.Error())
		}
	case s := <-signals:
		readinessProcess.Signal(s)
		<-readinessProcess.Wait()
		return ErrCancelled
	}

	step.logger.Info("transitioned-to-healthy")
	fmt.Fprint(step.logStreamer.Stdout(), "Container became healthy\n")
	close(ready)

	livenessProcess := ifrit.Background(step.livenessCheck)

	select {
	case err := <-livenessProcess.Wait():
		step.logger.Info("transitioned-to-unhealthy")
		fmt.Fprintf(step.healthCheckStreamer.Stderr(), "%s\n", err.Error())
		fmt.Fprint(step.logStreamer.Stdout(), "Container became unhealthy\n")
		return NewEmittableError(err, healthcheckNowUnhealthy, err.Error())
	case s := <-signals:
		livenessProcess.Signal(s)
		<-livenessProcess.Wait()
		return ErrCancelled
	}
}
