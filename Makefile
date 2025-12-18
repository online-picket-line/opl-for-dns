# Makefile for OPL DNS Plugin for BIND 9

# Plugin name
PLUGIN_NAME = opl-dns-plugin

# Directories
SRC_DIR = src
INC_DIR = include
BUILD_DIR = build

# Source files
SOURCES = $(SRC_DIR)/opl_plugin.c $(SRC_DIR)/bind_interface.c
OBJECTS = $(BUILD_DIR)/opl_plugin.o $(BUILD_DIR)/bind_interface.o

# Output
PLUGIN_SO = $(BUILD_DIR)/$(PLUGIN_NAME).so

# Compiler and flags
CC = gcc
CFLAGS = -Wall -Wextra -fPIC -I$(INC_DIR)
LDFLAGS = -shared

# BIND 9 includes and libraries
# These paths may need to be adjusted based on your BIND 9 installation
BIND_INC = /usr/include/bind9
BIND_LIB = /usr/lib/bind9

# Additional includes
CFLAGS += -I$(BIND_INC)

# Libraries
LIBS = -lcurl -ljson-c

# Targets
.PHONY: all clean install

all: $(PLUGIN_SO)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/%.o: $(SRC_DIR)/%.c | $(BUILD_DIR)
	$(CC) $(CFLAGS) -c $< -o $@

$(PLUGIN_SO): $(OBJECTS)
	$(CC) $(LDFLAGS) -o $@ $^ $(LIBS)
	@echo "Plugin built successfully: $(PLUGIN_SO)"

clean:
	rm -rf $(BUILD_DIR)

install: $(PLUGIN_SO)
	@echo "Installing plugin to /usr/lib/bind9/modules/"
	@echo "Note: You may need to run this with sudo"
	install -d /usr/lib/bind9/modules
	install -m 644 $(PLUGIN_SO) /usr/lib/bind9/modules/

# Help target
help:
	@echo "OPL DNS Plugin Build System"
	@echo ""
	@echo "Targets:"
	@echo "  all     - Build the plugin (default)"
	@echo "  clean   - Remove build artifacts"
	@echo "  install - Install the plugin to /usr/lib/bind9/modules/"
	@echo "  help    - Show this help message"
	@echo ""
	@echo "Requirements:"
	@echo "  - BIND 9 development headers"
	@echo "  - libcurl development headers"
	@echo "  - json-c development headers"
	@echo ""
	@echo "On Debian/Ubuntu, install with:"
	@echo "  sudo apt-get install bind9-dev libcurl4-openssl-dev libjson-c-dev"
