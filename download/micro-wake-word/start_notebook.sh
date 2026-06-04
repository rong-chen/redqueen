#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

echo "=== [microWakeWord] Starting Environment Setup ==="

# Get the directory of the script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

# 1. Create a virtual environment if it doesn't exist
if [ ! -d "venv" ]; then
    echo "Creating virtual environment 'venv'..."
    python3 -m venv venv
else
    echo "Virtual environment 'venv' already exists."
fi

# 2. Activate the virtual environment
echo "Activating virtual environment..."
source venv/bin/activate

# 3. Upgrade pip and install package dependencies
echo "Upgrading pip..."
pip install --upgrade pip

echo "Installing project dependencies..."
pip install -e .

echo "Installing Jupyter Notebook..."
pip install jupyter

echo "=== [microWakeWord] Setup Complete ==="
echo "Launching Jupyter Notebook..."

# 4. Launch Jupyter Notebook
jupyter notebook
