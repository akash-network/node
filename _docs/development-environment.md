# Setting up development environment

## Install dependencies
### macOS

> **WARNING**: macOS uses ancient version of the `make`. Akash's environment uses some tricks available in `make 4`.
We recommend use homebrew to installs most up-to-date version of the `make`. Keep in mind `make` is keg-only, and you'll need manually add its location to the `PATH`.
Make sure homebrew's make path takes precedence of `/usr/bin`


```shell
brew install curl wget jq direnv coreutils make npm

# Depending on your shell, it may go to .zshrc, .bashrc etc
export PATH="$(brew --prefix)/opt/make/libexec/gnubin:$PATH"
```

### Linux
#### Debian based
**TODO** validate
```shell
sudo apt update
sudo apt install -y jq curl wget build-essentials ca-certificates npm direnv gcc
```

## Direnv
Both [akash](https://github.com/akash-network/node) [provider-services](https://github.com/akash-network/provider) are extensively using `direnv` to setup and seamlessly update environment
while traversing across various directories. It is especially handy for running `provider-services` examples.

You may enable auto allow by whitelisting specific directories in `direnv.toml`.
To do so use following template to edit `${XDG_CONFIG_HOME:-$HOME/.config}/direnv/direnv.toml`
```toml
[whitelist]
prefix = [
    "<path to akash sources>",
    "<path to provider-services sources>"
]
```

## Cache

Build environment will create `.cache` directory in the root of source-tree. We use it to install specific versions of temporary build tools. Refer to `make/setup-cache.mk` for exact list.
It is possible to set custom path to `.cache` with `AKASH_DEVCACHE` environment variable.

All tools are referred as `makefile targets` and set as dependencies thus installed (to `.cache/bin`) only upon necessity.
For example `protoc` installed only when `proto-gen` target called.

The structure of the dir:
```shell
./cache
    bin/ # build tools
    run/ # work directories for _run examples (provider-services
    versions/ # versions of installed build tools (make targets use them to detect change of version of build tool and install new version if changed) 
```

### Add new tool

We will use `modevendor` as an example.
All variables must be capital case.

Following are added to `make/init.mk`
1. Add version variable as `<NAME>_VERSION ?= <version>` to the "# ==== Build tools versions ====" section
    ```makefile
    MODVENDOR_VERSION                  ?= v0.3.0
    ```
2. Add variable tracking version file `<NAME>_VERSION_FILE := $(AKASH_DEVCACHE_VERSIONS)/<tool>/$(<TOOL>)` to the `# ==== Build tools version tracking ====` section
    ```makefile
    MODVENDOR_VERSION_FILE             := $(AKASH_DEVCACHE_VERSIONS)/modvendor/$(MODVENDOR)
    ```
3. Add variable referencing executable to the `# ==== Build tools executables ====` section
    ```makefile
    MODVENDOR                          := $(AKASH_DEVCACHE_VERSIONS)/bin/modvendor
    ```

4. Add installation rules. Following template is used followed by the example
    ```makefile
    $(<TOOL>_VERSION_FILE): $(AKASH_DEVCACHE)
    	@echo "installing <tool> $(<TOOL>_VERSION) ..."
    	rm -f $(<TOOL>)      # remove current binary if exists
    	# installation procedure depends on distribution type. Check make/setup-cache.mk for various examples
    	rm -rf "$(dir $@)"   # remove current version file if exists
    	mkdir -p "$(dir $@)" # make new version directory
    	touch $@             # create new version file
    $(<TOOL>): $(<TOOL>_VERSION_FILE)
    ```

    Following are added to `make/setup-cache.mk`

    ```makefile
    $(MODVENDOR_VERSION_FILE): $(AKASH_DEVCACHE)
    	@echo "installing modvendor $(MODVENDOR_VERSION) ..."
    	rm -f $(MODVENDOR)
    	GOBIN=$(AKASH_DEVCACHE_BIN) $(GO) install github.com/goware/modvendor@$(MODVENDOR_VERSION)
    	rm -rf "$(dir $@)"
    	mkdir -p "$(dir $@)"
    	touch $@
    $(MODVENDOR): $(MODVENDOR_VERSION_FILE)
    ```
