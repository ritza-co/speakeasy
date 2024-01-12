# run  
`speakeasy run`  


run the workflow(s) defined in your `.speakeasy/workflow.yaml` file.  

## Details

run the workflow(s) defined in your `.speakeasy/workflow.yaml` file.
A workflow can consist of multiple targets that define a source OpenAPI document that can be downloaded from a URL, exist as a local file, or be created via merging multiple OpenAPI documents together and/or overlaying them with an OpenAPI overlay document.
A full workflow is capable of running the following steps:
  - Downloading source OpenAPI documents from a URL
  - Merging multiple OpenAPI documents together
  - Overlaying OpenAPI documents with an OpenAPI overlay document
  - Generating one or many SDKs from the resulting OpenAPI document
  - Compiling the generated SDKs

If `speakeasy run` is run without any arguments it will run either the first target in the workflow or the first source in the workflow if there are no other targets or sources, otherwise it will prompt you to select a target or source to run.

## Usage

```
speakeasy run [flags]
```

### Options

```
  -d, --debug                    enable writing debug files with broken code
  -h, --help                     help for run
  -i, --installationURL string   the language specific installation URL for installation instructions if the SDK is not published to a package manager
  -r, --repo string              the repository URL for the SDK, if the published (-p) flag isn't used this will be used to generate installation instructions
  -b, --repo-subdir string       the subdirectory of the repository where the SDK is located in the repo, helps with documentation generation
  -s, --source string            source to run. specify 'all' to run all sources
  -t, --target string            target to run. specify 'all' to run all targets
```

### Parent Command

* [speakeasy](README.md)	 - The speakeasy cli tool provides access to the speakeasyapi.dev toolchain