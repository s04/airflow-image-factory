import streamlit as st
from urllib.parse import urljoin
import requests
import hashlib
import json
import os
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()


def read_extras_from_file(filename):
    with open(filename, "r") as f:
        extras = [
            line.strip() for line in f if line.strip() and not line.startswith("#")
        ]
    return extras


def generate_dockerfile(
    airflow_version,
    python_version,
    base_image,
    extras,
    apt_deps,
    pip_deps,
    custom_airflow_cfg=None,
):
    dockerfile = f"""
FROM apache/airflow:{airflow_version}-python{python_version}

USER root

# Install apt dependencies
RUN apt-get update && apt-get install -y --no-install-recommends {' '.join(apt_deps)} && \\
    apt-get autoremove -yqq --purge && \\
    apt-get clean && \\
    rm -rf /var/lib/apt/lists/*

USER airflow

# Install Airflow with extras and additional pip dependencies
RUN pip install --no-cache-dir "apache-airflow[{','.join(extras)}]=={airflow_version}" {' '.join(pip_deps)}

"""
    if custom_airflow_cfg:
        dockerfile += """
# Copy custom airflow.cfg
COPY airflow.cfg /opt/airflow/airflow.cfg
"""

    dockerfile += """
CMD ["airflow"]
"""
    return dockerfile


def send_build_request(build_params):
    api_url = "http://172.17.0.1:8081/build-and-push"
    try:
        response = requests.post(api_url, json=build_params)
        print(f"Request sent: {response.request.url}")
        print(f"Request body: {response.request.body}")
        st.sidebar.write(f"Request sent: {response.request.url}")
        st.sidebar.write(f"Request body: {response.request.body}")  # Corrected line
        response.raise_for_status()
        return response.json()
    except requests.exceptions.RequestException as e:
        st.error(f"Error sending build request: {str(e)}")
        return None


st.title("Airflow Dockerfile Generator")

airflow_version = st.text_input("Airflow version", "2.9.3")
python_version = st.selectbox("Python version", ["3.8", "3.9", "3.10", "3.11"])
base_image = st.selectbox("Base image", ["slim", "bookworm", "bullseye"])

# Read extras from file
all_extras = read_extras_from_file("airflow_extras.txt")

# Use a multiselect for extras
extras = st.multiselect("Select Airflow extras", all_extras)

apt_deps = st.text_area("APT dependencies (one per line)")
pip_deps = st.text_area("Additional pip dependencies (one per line)")

custom_airflow_cfg = st.text_area("Custom airflow.cfg content (optional)")

col1, col2 = st.columns(2)

if col1.button("Generate Dockerfile"):
    apt_deps_list = [dep.strip() for dep in apt_deps.split("\n") if dep.strip()]
    pip_deps_list = [dep.strip() for dep in pip_deps.split("\n") if dep.strip()]

    dockerfile = generate_dockerfile(
        airflow_version,
        python_version,
        base_image,
        extras,
        apt_deps_list,
        pip_deps_list,
        custom_airflow_cfg,
    )

    st.subheader("Generated Dockerfile")
    st.code(dockerfile, language="dockerfile")

if col2.button("Build and Push Image"):
    apt_deps_list = [dep.strip() for dep in apt_deps.split("\n") if dep.strip()]
    pip_deps_list = [dep.strip() for dep in pip_deps.split("\n") if dep.strip()]

    build_params = {
        "airflow_version": airflow_version,
        "python_version": python_version,
        "base_image": base_image,
        "extras": extras,
        "apt_deps": apt_deps_list,
        "pip_deps": pip_deps_list,
    }

    with st.spinner("Building and pushing Docker image..."):
        result = send_build_request(build_params)
        if result:
            st.success(result["message"])
            st.info(f"Image tag: {result.get('image_tag', 'N/A')}")
        else:
            st.error(
                "Failed to build and push Docker image. Please check the logs for details."
            )

st.sidebar.header("About")
st.sidebar.info(
    "This app generates a Dockerfile for Apache Airflow and can build and push the image to a Docker registry. "
    "Customize the Airflow version, Python version, extras, and dependencies to create your perfect Airflow image."
)
