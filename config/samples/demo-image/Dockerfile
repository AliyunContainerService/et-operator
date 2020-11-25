FROM horovod/horovod:0.19.3-tf2.1.0-torch-mxnet1.6.0-py3.6-gpu

# install horovod from source code
RUN pip install --upgrade pip
RUN mkdir -p /tmp && \
    cd /tmp/ && \
    git clone --recursive https://github.com/horovod/horovod.git && \
    cd horovod && \
    pip uninstall -y horovod && \
    rm -rf build/ dist/  && \
    export LD_LIBRARY_PATH=/usr/local/cuda:/usr/local/cuda/compat/:${LD_LIBRARY_PATH} && \
    HOROVOD_GPU_OPERATIONS=NCCL \
    HOROVOD_WITH_TENSORFLOW=1 \
    HOROVOD_WITH_PYTORCH=1 \
    HOROVOD_WITH_MXNET=1 \
    HOROVOD_WITH_GLOO=1 \
    python setup.py install && \
    rm -rf /tmp/*

# install dependency of horovod
RUN pip install dataclasses==0.7

ADD discover_hosts.sh /usr/local/bin/discover_hosts.sh
ADD scaler.sh /usr/local/bin/scaler.sh

RUN chmod +x /usr/local/bin/discover_hosts.sh && \
    chmod +x /usr/local/bin/scaler.sh

# replace dash by bash for sh
RUN ln -sf /bin/bash /bin/sh