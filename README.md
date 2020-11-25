# Elastic Training Operator

## Overview

Some distributed deep learning training framework like [horovod](https://github.com/horovod/horovod)  support elastic training, which enables training job scale up and down the number of workers dynamically at runtime without interrupting the training process.

Et-operator provides a set of Kubernetes Custom Resource Definition that makes it easy to run horovod or AIACC elastic training in kubernetes. After submit a training job, you can scaleIn and scaleOut workers during training on demand, which can make your training job more elasticity and efficient.


## Design

For more details about the design of this project, please read this Design document.


## Setup
### Installation

```
git clone https://http://github.com/aliyunContainerService/et-operator
cd et-operator
kubectl create -f config/deploy.yaml
```

Or you can customize some config, and run:

```
make deploy
```

You can check whether the Training Job custom resource is installed via:

```
kubectl get crd

```

```
NAME                                    CREATED AT
scaleins.kai.alibabacloud.com           2020-11-11T11:16:13Z
scaleouts.kai.alibabacloud.com          2020-11-11T11:16:13Z
trainingjobs.kai.alibabacloud.com       2020-11-11T11:16:13Z
```

Check the operator status

```
kubectl get pod -n kube-ai
```

```
NAME                                          READY   STATUS    RESTARTS   AGE
et-operator-controller-manager-6d4688cf58-wvhbr   1/1     Running   0          5m

```


## User guide

### Create a elastic training job
The training code need to be constructed in in elastic training mod,  [see detail](https://horovod.readthedocs.io/en/stable/elastic_include.html).
You can create an Training job by submit an TrainingJob YAML file. You can goto [Horovod TrainingJob Example](./example/training_job.yaml) to see the example, and you can modify it in need.


```
kubectl apply -f examples/training_job.yaml

```

#### Check TrainingJob status

```
# kubectl get trainingjob
NAME                          PHASE     AGE
elastic-training              Running   77s
```

```
# kubectl get po
NAME                                      READY   STATUS             RESTARTS   AGE
elastic-training-launcher                 1/1     Running            0          7s
elastic-training-worker-0                 1/1     Running            0          10s
elastic-training-worker-1                 1/1     Running            0          9s
```


### ScaleIn training job
When you need to scaleIn the trainingJob workers, you can submit an ScaleIn CustomResource.
In `Scalein` Spec, you need to spec the name of TrainingJob, et-operator will find the match trainingJob and execute scaleIn to it. You can specify the workers to scaleIn [ScaleIn by count](./example/scale_in_pod.yaml) or just specify the count [ScaleIn by count](./example/scale_in_count.yaml) .

```
kubectl create -f examples/scale_in_count.yaml


```
#### Check Scalein status

```
# kubectl get scalein
NAME                                     PHASE            AGE
scalein-sample-t8jxd                     ScaleSucceeded   11s
```


```
# kubectl get po
NAME                                      READY   STATUS             RESTARTS   AGE
elastic-training-launcher                 1/1     Running            0          47s
elastic-training-worker-0                 1/1     Running            0          50s
```

### ScaleOut training job
When you need to scaleOut the trainingJob workers, you can submit an ScaleOut CustomResource, which just specify the count of workers you want to scaleOut.

```
kubectl create -f examples/scale_out.yaml

```
#### Check ScaleOut status

```
# kubectl get scaleout
NAME                                     PHASE            AGE
elastic-training-scaleout-9dtmw          ScaleSucceeded   30s

```

```
# kubectl get po
NAME                                      READY   STATUS             RESTARTS   AGE
elastic-training-launcher                 1/1     Running            0          2m5s
elastic-training-worker-0                 1/1     Running            0          2m8s
elastic-training-worker-1                 1/1     Running            0          40s
elastic-training-worker-2                 1/1     Running            0          40s
```


## Roadmap

* Support fault tolerance
* Support spot instance in public cloud, before node released, trigger a scalein to the training worker that on the node will be relased.

## Developing
Prerequisites:

* Go >= 1.8
* kubebuilder >= 0.4.1

```
mkdir -p $(go env GOPATH)/src/github.com/aliyunContainerService
cd $(go env GOPATH)/src/github.com/aliyunContainerService
git clone https://http://github.com/aliyunContainerService/et-operator
cd et-operator
make
```

Build operator

```
export IMG=<image repo>
make docker-build
make docker-push

```


Running operator in local

```
make run-local
```

