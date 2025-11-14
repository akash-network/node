include $(abspath $(CURDIR)/../../make/init.mk)

ifeq ($(AKASH_RUN_NAME),)
$(error "AKASH_RUN_NAME is not set")
endif

ifeq ($(AKASH_RUN_DIR),)
$(error "AKASH_RUN_DIR is not set")
endif

ifneq ($(AKASH_HOME),)
ifneq ($(DIRENV_FILE),$(CURDIR)/.envrc)
$(error "AKASH_HOME is set by the upper dir (probably in ~/.bashrc|~/.zshrc), \
but direnv does not seem to be configured. \
Ensure direnv is installed and hooked to your shell profile. Refer to the documentation for details. \
")
endif
else
$(error "AKASH_HOME is not set")
endif

.PHONY: akash
akash:
ifneq ($(SKIP_BUILD), true)
	make -C $(AKASH_ROOT) akash
endif

.PHONY: bins
bins: akash
