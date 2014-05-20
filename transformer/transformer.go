package transformer

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cloudfoundry-incubator/executor/log_streamer_factory"
	"github.com/cloudfoundry-incubator/executor/sequence"
	"github.com/cloudfoundry-incubator/executor/steps/download_step"
	"github.com/cloudfoundry-incubator/executor/steps/emit_progress_step"
	"github.com/cloudfoundry-incubator/executor/steps/fetch_result_step"
	"github.com/cloudfoundry-incubator/executor/steps/monitor_step"
	"github.com/cloudfoundry-incubator/executor/steps/parallel_step"
	"github.com/cloudfoundry-incubator/executor/steps/run_step"
	"github.com/cloudfoundry-incubator/executor/steps/try_step"
	"github.com/cloudfoundry-incubator/executor/steps/upload_step"
	"github.com/cloudfoundry-incubator/executor/uploader"
	"github.com/cloudfoundry-incubator/garden/warden"
	"github.com/cloudfoundry-incubator/runtime-schema/models"
	steno "github.com/cloudfoundry/gosteno"
	"github.com/pivotal-golang/archiver/compressor"
	"github.com/pivotal-golang/archiver/extractor"
	"github.com/pivotal-golang/cacheddownloader"
)

var ErrNoInterval = errors.New("no interval configured")
var ErrNoCheck = errors.New("no check configured")

type Transformer struct {
	logStreamerFactory log_streamer_factory.LogStreamerFactory
	cachedDownloader   cacheddownloader.CachedDownloader
	uploader           uploader.Uploader
	extractor          extractor.Extractor
	compressor         compressor.Compressor
	logger             *steno.Logger
	tempDir            string
	result             *string
}

func NewTransformer(
	logStreamerFactory log_streamer_factory.LogStreamerFactory,
	cachedDownloader cacheddownloader.CachedDownloader,
	uploader uploader.Uploader,
	extractor extractor.Extractor,
	compressor compressor.Compressor,
	logger *steno.Logger,
	tempDir string,
) *Transformer {
	return &Transformer{
		logStreamerFactory: logStreamerFactory,
		cachedDownloader:   cachedDownloader,
		uploader:           uploader,
		extractor:          extractor,
		compressor:         compressor,
		logger:             logger,
		tempDir:            tempDir,
	}
}

func (transformer *Transformer) StepsFor(
	logConfig models.LogConfig,
	actions []models.ExecutorAction,
	container warden.Container,
	result *string,
) ([]sequence.Step, error) {
	subSteps := []sequence.Step{}

	for _, a := range actions {
		step, err := transformer.convertAction(logConfig, a, container, result)
		if err != nil {
			return nil, err
		}

		subSteps = append(subSteps, step)
	}

	return subSteps, nil
}

func (transformer *Transformer) convertAction(
	logConfig models.LogConfig,
	action models.ExecutorAction,
	container warden.Container,
	result *string,
) (sequence.Step, error) {
	logStreamer := transformer.logStreamerFactory(logConfig)

	switch actionModel := action.Action.(type) {
	case models.RunAction:
		return run_step.New(
			container,
			actionModel,
			logStreamer,
			transformer.logger,
		), nil
	case models.DownloadAction:
		return download_step.New(
			container,
			actionModel,
			transformer.cachedDownloader,
			transformer.extractor,
			transformer.tempDir,
			transformer.logger,
		), nil
	case models.UploadAction:
		return upload_step.New(
			container,
			actionModel,
			transformer.uploader,
			transformer.compressor,
			transformer.tempDir,
			logStreamer,
			transformer.logger,
		), nil
	case models.FetchResultAction:
		return fetch_result_step.New(
			container,
			actionModel,
			transformer.tempDir,
			transformer.logger,
			result,
		), nil
	case models.EmitProgressAction:
		subStep, err := transformer.convertAction(
			logConfig,
			actionModel.Action,
			container,
			result,
		)
		if err != nil {
			return nil, err
		}

		return emit_progress_step.New(
			subStep,
			actionModel.StartMessage,
			actionModel.SuccessMessage,
			actionModel.FailureMessage,
			logStreamer,
			transformer.logger,
		), nil
	case models.TryAction:
		subStep, err := transformer.convertAction(
			logConfig,
			actionModel.Action,
			container,
			result,
		)
		if err != nil {
			return nil, err
		}

		return try_step.New(subStep, transformer.logger), nil
	case models.MonitorAction:
		healthyHookURL, err := url.ParseRequestURI(actionModel.HealthyHook.URL)
		if err != nil {
			return nil, err
		}

		unhealthyHookURL, err := url.ParseRequestURI(actionModel.UnhealthyHook.URL)
		if err != nil {
			return nil, err
		}

		if actionModel.Interval == 0 {
			return nil, ErrNoInterval
		}

		check, err := transformer.convertAction(
			logConfig,
			actionModel.Action,
			container,
			result,
		)
		if err != nil {
			return nil, err
		}

		return monitor_step.New(
			check,
			actionModel.Interval,
			actionModel.HealthyThreshold,
			actionModel.UnhealthyThreshold,
			http.Request{
				Method: actionModel.HealthyHook.Method,
				URL:    healthyHookURL,
			},
			http.Request{
				Method: actionModel.UnhealthyHook.Method,
				URL:    unhealthyHookURL,
			},
		), nil
	case models.ParallelAction:
		steps := make([]sequence.Step, len(actionModel.Actions))
		for i, action := range actionModel.Actions {
			var err error

			steps[i], err = transformer.convertAction(
				logConfig,
				action,
				container,
				result,
			)
			if err != nil {
				return nil, err
			}
		}

		return parallel_step.New(steps), nil
	}

	panic(fmt.Sprintf("unknown action: %T", action))
}