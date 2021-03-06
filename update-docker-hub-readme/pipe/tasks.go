package pipe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	utils "github.com/cenk1cenk2/ci-cd-pipes/utils"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

var DockerClient *client.Client

type Ctx struct {
	token string
}

var Context Ctx

func TaskVerifyVariables() utils.Task {
	metadata := utils.TaskMetadata{Context: "Verify"}

	return utils.Task{Metadata: metadata, Task: func(t *utils.Task) error {
		log := utils.Log.WithField("context", t.Metadata.Context)

		if len(Pipe.Readme.Description) > 100 {
			log.Fatalln(
				fmt.Sprintf(
					"Readme short description can only be 100 characters long while you have: %d",
					len(Pipe.Readme.Description),
				),
			)
		}

		return nil
	}}
}

func TaskLoginToDockerHubRegistry() utils.Task {
	metadata := utils.TaskMetadata{Context: "DockerHub - login"}

	return utils.Task{Metadata: metadata, Task: func(t *utils.Task) error {
		log := utils.Log.WithField("context", t.Metadata.Context)

		login, err := json.Marshal(types.AuthConfig{
			Username: Pipe.DockerHub.Username,
			Password: Pipe.DockerHub.Password,
		})

		if err != nil {
			return err
		}

		res, err := http.Post(
			"https://hub.docker.com/v2/users/login/",
			JSON_REQUEST,
			bytes.NewReader(login),
		)

		if err != nil {
			return err
		}

		log.Debugln("Authentication token obtained.")

		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)

		if err != nil {
			return err
		}

		b := DockerHubLoginResponse{}
		err = json.Unmarshal(body, &b)

		Context.token = b.Token

		return nil

	}}
}

func TaskUpdateDockerReadme() utils.Task {
	metadata := utils.TaskMetadata{Context: "DockerHub - update readme"}

	return utils.Task{Metadata: metadata, Task: func(t *utils.Task) error {
		log := utils.Log.WithField("context", t.Metadata.Context)

		log.Debugln(fmt.Sprintf("Trying to read file: %s", Pipe.Readme.File))

		content, err := ioutil.ReadFile(Pipe.Readme.File)

		if err != nil {
			return err
		}

		readme := string(content)

		log.Debugln(fmt.Sprintf("File read: %s", Pipe.Readme.File))
		log.Debugln(
			fmt.Sprintf(
				"Running against repository: %s/%s",
				Pipe.DockerHub.Address,
				Pipe.Readme.Repository,
			),
		)

		update := DockerHubUpdateReadmeRequest{
			Readme: readme,
		}

		if Pipe.Readme.Description != "" {
			update.Description = Pipe.Readme.Description
		}

		body, err := json.Marshal(
			update,
		)

		if err != nil {
			return err
		}

		req, err := http.NewRequest(http.MethodPatch,
			fmt.Sprintf("%s/%s/", Pipe.DockerHub.Address, Pipe.Readme.Repository),
			bytes.NewReader(body),
		)

		req = addAuthenticationHeadersToRequest(req)

		if err != nil {
			return err
		}

		res, err := http.DefaultClient.Do(req)

		if err != nil {
			return err
		}

		log.Debugln(fmt.Sprintf("Status Code: %d", res.StatusCode))

		defer res.Body.Close()

		body, err = ioutil.ReadAll(res.Body)

		if err != nil {
			return err
		}

		log.Debugln(fmt.Sprintf("Response body: %s", string(body)))

		b := DockerHubUpdateReadmeResponse{}
		err = json.Unmarshal(body, &b)

		if err != nil {
			log.Errorln(fmt.Sprintf("Response unexpected: %s", string(body)))

			return err
		}

		switch res.StatusCode {
		case 200:
			if b.FullDescription != readme {
				log.Fatalln("Uploaded README does not match with current repository README file.")
			}

			if Pipe.Readme.Description != "" && b.Description != Pipe.Readme.Description {
				log.Fatalln("Uploaded README does not match with current repository README file.")
			}

			log.Infoln(
				fmt.Sprintf(
					"Successfully pushed readme file to: %s -> %s/%s",
					Pipe.Readme.File,
					Pipe.DockerHub.Address,
					Pipe.Readme.Repository,
				),
			)
		case 404:
			log.Fatalln(
				fmt.Sprintf(
					"Repository does not exists: %s/%s",
					Pipe.DockerHub.Address,
					Pipe.Readme.Repository,
				),
			)
		default:
			if !b.CanEdit {
				log.Fatalln(
					fmt.Sprintf(
						"Given user credentials do not have permission to edit repository: %s/%s",
						Pipe.DockerHub.Address,
						Pipe.Readme.Repository,
					),
				)
			}

			log.Fatalln(
				fmt.Sprintf(
					"Pushing readme failed with code: %d",
					res.StatusCode,
				),
			)
		}

		return nil
	}}
}

func addAuthenticationHeadersToRequest(req *http.Request) *http.Request {
	req.Header.Add("User-Agent", CLI_NAME)
	req.Header.Add("Content-Type", JSON_REQUEST)
	req.Header.Add("Authorization", fmt.Sprintf("JWT %s", Context.token))

	return req
}
