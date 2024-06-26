package prompts

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/speakeasy-api/huh"
	"github.com/speakeasy-api/sdk-gen-config/workflow"
	"github.com/speakeasy-api/speakeasy-core/auth"
	charm_internal "github.com/speakeasy-api/speakeasy/internal/charm"
	"github.com/speakeasy-api/speakeasy/registry"
)

func getBaseSourcePrompts(currentWorkflow *workflow.Workflow, sourceName, fileLocation, authHeader *string) []*huh.Group {
	var initialGroup []huh.Field

	if fileLocation == nil || *fileLocation == "" {
		initialGroup = append(initialGroup,
			charm_internal.NewInput().
				Title("What is the location of your OpenAPI document?").
				Placeholder("local file path or remote file reference.").
				Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
				SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
					FileExtensions: charm_internal.OpenAPIFileExtensions,
				})).
				Value(fileLocation),
		)
	}

	if sourceName == nil || *sourceName == "" {
		initialGroup = append(initialGroup,
			charm_internal.NewInput().
				Title("What is a good name for this source document?").
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("a source name must be provided")
					}

					if strings.Contains(s, " ") {
						return fmt.Errorf("a source name must not contain spaces")
					}

					if _, ok := currentWorkflow.Sources[s]; ok {
						return fmt.Errorf("a source with the name %s already exists", s)
					}
					return nil
				}).
				Value(sourceName),
		)
	}

	var groups []*huh.Group

	if len(initialGroup) > 0 {
		groups = append(groups, huh.NewGroup(initialGroup...))
	}

	groups = append(groups, getRemoteAuthenticationPrompts(fileLocation, authHeader)...)
	return groups
}

func getRemoteAuthenticationPrompts(fileLocation, authHeader *string) []*huh.Group {
	requiresAuthentication := false
	return []*huh.Group{
		huh.NewGroup(
			huh.NewConfirm().
				Title("Does this remote file require authentication?").
				Affirmative("Yes.").
				Negative("No.").
				Value(&requiresAuthentication),
		).WithHideFunc(func() bool {
			if fileLocation != nil && *fileLocation != "" {
				if parsedUrl, err := url.ParseRequestURI(*fileLocation); err == nil {
					resp, err := http.Get(parsedUrl.String())
					if err != nil {
						return false
					} else {
						defer resp.Body.Close()

						if resp.StatusCode < 200 || resp.StatusCode > 299 {
							return false
						}
					}
				}
			}
			return true
		}),
		huh.NewGroup(
			charm_internal.NewInput().
				Title("What is the name of your authentication header?").
				Description("The value for this header will be fetched from the secret $OPENAPI_DOC_AUTH_TOKEN\n").
				Inline(false).
				Prompt("").
				Placeholder("x-auth-token").
				Value(authHeader),
		).WithHideFunc(func() bool {
			return !requiresAuthentication
		}),
	}
}

func getOverlayPrompts(promptForOverlay *bool, overlayLocation, authHeader *string) []*huh.Group {
	groups := []*huh.Group{
		huh.NewGroup(
			charm_internal.NewInput().
				Title("What is the location of your Overlay file?").
				Placeholder("local file path or remote file reference.").
				Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
				SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
					FileExtensions: charm_internal.OpenAPIFileExtensions,
				})).
				Value(overlayLocation),
		).WithHideFunc(func() bool {
			return !*promptForOverlay
		}),
	}

	groups = append(groups, getRemoteAuthenticationPrompts(overlayLocation, authHeader)...)
	return groups
}

func sourceBaseForm(ctx context.Context, quickstart *Quickstart) (*QuickstartState, error) {
	source := &workflow.Source{}
	var sourceName, fileLocation, authHeader string

	if quickstart.Defaults.SchemaPath != nil {
		fileLocation = *quickstart.Defaults.SchemaPath
	}

	useSampleSpec := false
	_, err := charm_internal.NewForm(huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[bool]().
				Title("Do you have an existing OpenAPI spec?").
				Description("You can provide a local file path or a remote file URL to your OpenAPI spec.").
				Options(
					huh.NewOption("Yes", false),
					huh.NewOption("No, use a sample OpenAPI spec", true),
				).
				Value(&useSampleSpec),
		),
	)).ExecuteForm()
	if err != nil {
		return nil, err
	}

	if useSampleSpec {
		quickstart.IsUsingSampleOpenAPISpec = true
		// Other parts of the code make assumptions that the workflow has a valid source
		// This is a hack to satisfy those assumptions, we will overwrite this with a proper
		// file location when we have written the sample spec to disk when we know the SDK output directory
		fileLocation = "https://example.com/OVERWRITE_WHEN_SAMPLE_SPEC_IS_WRITTEN"
	} else {
		if _, err := charm_internal.NewForm(huh.NewForm(
			getBaseSourcePrompts(quickstart.WorkflowFile, &sourceName, &fileLocation, &authHeader)...),
			"Let's setup a new source for your workflow.",
			"A source is a compiled set of OpenAPI specs and overlays that are used as the input for a SDK generation.").
			ExecuteForm(); err != nil {
			return nil, err
		}
	}

	document, err := formatDocument(fileLocation, authHeader, false)
	if err != nil {
		return nil, err
	}

	source.Inputs = append(source.Inputs, *document)

	if registry.IsRegistryEnabled(ctx) && auth.GetOrgSlugFromContext(ctx) != "" && auth.GetWorkspaceSlugFromContext(ctx) != "" {
		registryEntry := &workflow.SourceRegistry{}
		if err := registryEntry.SetNamespace(fmt.Sprintf("%s/%s/%s", auth.GetOrgSlugFromContext(ctx), auth.GetWorkspaceSlugFromContext(ctx), strcase.ToKebab(sourceName))); err != nil {
			return nil, err
		}
		source.Registry = registryEntry
	}

	if err := source.Validate(); err != nil {
		return nil, errors.Wrap(err, "failed to validate source")
	}

	quickstart.WorkflowFile.Sources[sourceName] = *source

	nextState := TargetBase

	return &nextState, nil
}

func AddToSource(name string, currentSource *workflow.Source) (*workflow.Source, error) {
	addOpenAPIFile := false
	var inputOptions []huh.Option[string]
	for _, option := range getCurrentInputs(currentSource) {
		inputOptions = append(inputOptions, huh.NewOption(charm_internal.FormatEditOption(option), option))
	}
	inputOptions = append(inputOptions, huh.NewOption(charm_internal.FormatNewOption("New Document"), "new document"))
	selectedDoc := ""
	prompt := charm_internal.NewSelectPrompt("Would you like to modify the location of an existing OpenAPI document or add a new one?", "", inputOptions, &selectedDoc)
	if _, err := charm_internal.NewForm(huh.NewForm(
		prompt),
		fmt.Sprintf("Let's modify the source %s", name)).
		ExecuteForm(); err != nil {
		return nil, err
	}

	addOpenAPIFile = selectedDoc == "new document"
	if !addOpenAPIFile {
		fileLocation := selectedDoc
		var authHeader string
		groups := []*huh.Group{
			huh.NewGroup(
				charm_internal.NewInput().
					Title("What is the location of your OpenAPI document?\n").
					Placeholder("local file path or remote file reference.").
					Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
					SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
						FileExtensions: charm_internal.OpenAPIFileExtensions,
					})).
					Inline(false).
					Value(&fileLocation),
			),
		}
		groups = append(groups, getRemoteAuthenticationPrompts(&fileLocation, &authHeader)...)
		if _, err := charm_internal.NewForm(huh.NewForm(
			groups...),
			fmt.Sprintf("Let's modify the source %s", name)).
			ExecuteForm(); err != nil {
			return nil, err
		}

		for index, input := range currentSource.Inputs {
			if input.Location == selectedDoc {
				newInput := workflow.Document{}
				newInput.Location = fileLocation
				if authHeader != "" {
					newInput.Auth = &workflow.Auth{
						Header: authHeader,
						Secret: "$openapi_doc_auth_token",
					}
				}
				currentSource.Inputs[index] = newInput
				break
			}
		}
	}

	for addOpenAPIFile {
		addOpenAPIFile = false
		var fileLocation, authHeader string
		groups := []*huh.Group{
			huh.NewGroup(
				charm_internal.NewInput().
					Title("What is the location of your OpenAPI document?").
					Placeholder("local file path or remote file reference.").
					Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
					SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
						FileExtensions: charm_internal.OpenAPIFileExtensions,
					})).
					Value(&fileLocation),
			),
		}
		groups = append(groups, getRemoteAuthenticationPrompts(&fileLocation, &authHeader)...)
		groups = append(groups, charm_internal.NewBranchPrompt("Would you like to add another openapi file to this source?", &addOpenAPIFile))
		if _, err := charm_internal.NewForm(huh.NewForm(
			groups...),
			fmt.Sprintf("Let's add to the source %s", name)).
			ExecuteForm(); err != nil {
			return nil, err
		}
		document, err := formatDocument(fileLocation, authHeader, true)
		if err != nil {
			return nil, err
		}

		currentSource.Inputs = append(currentSource.Inputs, *document)
	}

	addOverlayFile := false
	if _, err := charm_internal.NewForm(huh.NewForm(
		charm_internal.NewBranchPrompt("Would you like to add an overlay file to this source?", &addOverlayFile)),
		fmt.Sprintf("Let's add to the source %s", name)).
		ExecuteForm(); err != nil {
		return nil, err
	}

	for addOverlayFile {
		addOverlayFile = false
		var fileLocation, authHeader string
		trueVal := true
		groups := getOverlayPrompts(&trueVal, &fileLocation, &authHeader)
		groups = append(groups, charm_internal.NewBranchPrompt("Would you like to add another overlay file to this source?", &addOverlayFile))
		if _, err := charm_internal.NewForm(huh.NewForm(
			groups...),
			fmt.Sprintf("Let's add to the source %s", name)).
			ExecuteForm(); err != nil {
			return nil, err
		}
		document, err := formatDocument(fileLocation, authHeader, true)
		if err != nil {
			return nil, err
		}

		currentSource.Overlays = append(currentSource.Overlays, *document)
	}

	if len(currentSource.Inputs)+len(currentSource.Overlays) > 1 {
		outputLocation := ""
		if currentSource.Output != nil {
			outputLocation = *currentSource.Output
		}

		previousOutputLocation := outputLocation
		if _, err := charm_internal.NewForm(huh.NewForm(
			huh.NewGroup(
				charm_internal.NewInput().
					Title("Optionally provide an output location for your build source file:").
					Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
					SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
						FileExtensions: charm_internal.OpenAPIFileExtensions,
					})).
					Value(&outputLocation),
			)),
			fmt.Sprintf("Let's modify the source %s", name)).
			ExecuteForm(); err != nil {
			return nil, err
		}

		if previousOutputLocation != outputLocation {
			currentSource.Output = &outputLocation
		}
	}

	if err := currentSource.Validate(); err != nil {
		return nil, errors.Wrap(err, "failed to validate source")
	}

	return currentSource, nil
}

func PromptForNewSource(currentWorkflow *workflow.Workflow) (string, *workflow.Source, error) {
	source := &workflow.Source{}
	var sourceName, fileLocation, authHeader string
	var overlayFileLocation, overlayAuthHeader, outputLocation string

	groups := getBaseSourcePrompts(currentWorkflow, &sourceName, &fileLocation, &authHeader)
	var promptForOverlay bool
	groups = append(groups, charm_internal.NewBranchPrompt("Would you like to add an overlay file to this source?", &promptForOverlay))
	groups = append(groups, getOverlayPrompts(&promptForOverlay, &overlayFileLocation, &overlayAuthHeader)...)
	groups = append(groups, huh.NewGroup(
		charm_internal.NewInput().
			Title("Optionally provide an output location for your build source file:").
			Placeholder("output.yaml").
			Suggestions(charm_internal.SchemaFilesInCurrentDir("", charm_internal.OpenAPIFileExtensions)).
			SetSuggestionCallback(charm_internal.SuggestionCallback(charm_internal.SuggestionCallbackConfig{
				FileExtensions: charm_internal.OpenAPIFileExtensions,
			})).
			Value(&outputLocation),
	).WithHideFunc(
		func() bool {
			return overlayFileLocation == ""
		}))

	if _, err := charm_internal.NewForm(huh.NewForm(
		groups...),
		"Let's setup a new source for your workflow.",
		"A source is a compiled set of OpenAPI specs and overlays that are used as the input for a SDK generation.").
		ExecuteForm(); err != nil {
		return "", nil, err
	}

	document, err := formatDocument(fileLocation, authHeader, false)
	if err != nil {
		return "", nil, err
	}

	source.Inputs = append(source.Inputs, *document)

	if overlayFileLocation != "" {
		document, err := formatDocument(overlayFileLocation, overlayAuthHeader, false)
		if err != nil {
			return "", nil, err
		}

		source.Overlays = append(source.Overlays, *document)
	}

	if outputLocation != "" {
		source.Output = &outputLocation
	}

	if err := source.Validate(); err != nil {
		return "", nil, errors.Wrap(err, "failed to validate source")
	}

	return sourceName, source, nil
}

func formatDocument(fileLocation, authHeader string, validate bool) (*workflow.Document, error) {
	if strings.Contains(fileLocation, "github.com") {
		fileLocation = strings.Replace(fileLocation, "github.com", "raw.githubusercontent.com", 1)
		fileLocation = strings.Replace(fileLocation, "/blob/", "/", 1)
	}

	document := &workflow.Document{
		Location: fileLocation,
	}

	if authHeader != "" {
		document.Auth = &workflow.Auth{
			Header: authHeader,
			Secret: "$openapi_doc_auth_token",
		}
	}

	if validate {
		if err := document.Validate(); err != nil {
			return nil, errors.Wrap(err, "failed to validate new document")
		}
	}

	return document, nil
}
