pipeline {
    agent any

    environment {
        REGISTRY     = 'ghcr.io'
        IMAGE_BASE   = "${REGISTRY}/opalwight/the-ranch"
        GIT_SHA      = sh(script: 'git rev-parse HEAD', returnStdout: true).trim()
        GIT_SHA_SHORT = sh(script: 'git rev-parse --short HEAD', returnStdout: true).trim()
    }

    options {
        timestamps()
        ansiColor('xterm')
        disableConcurrentBuilds()
        buildDiscarder(logRotator(numToKeepStr: '20'))
    }

    stages {

        // ───────────────────── CI ─────────────────────

        stage('Test') {
            steps {
                sh '''
                    echo "Starting test Postgres..."
                    docker compose -f docker-compose.yaml up -d postgres
                    sleep 5

                    # Run migrations
                    for f in migrations/*.up.sql; do
                        echo "Running $f..."
                        docker compose exec -T postgres \
                            psql -U filesync -d filesync < "$f"
                    done

                    # Run tests and vet
                    go test -race ./...
                    go vet ./...

                    docker compose -f docker-compose.yaml stop postgres
                '''
            }
        }

        stage('Build Images') {
            parallel {
                stage('API') {
                    steps {
                        sh """
                            docker build --target api \
                                -t ${IMAGE_BASE}/api:${GIT_SHA} \
                                -t ${IMAGE_BASE}/api:latest \
                                .
                        """
                    }
                }
                stage('Worker') {
                    steps {
                        sh """
                            docker build --target worker \
                                -t ${IMAGE_BASE}/worker:${GIT_SHA} \
                                -t ${IMAGE_BASE}/worker:latest \
                                .
                        """
                    }
                }
                stage('Web') {
                    steps {
                        sh """
                            docker build \
                                -t ${IMAGE_BASE}/web:${GIT_SHA} \
                                -t ${IMAGE_BASE}/web:latest \
                                ./web
                        """
                    }
                }
            }
        }

        stage('Push Images') {
            steps {
                withCredentials([usernamePassword(
                    credentialsId: 'ghcr-credentials',
                    usernameVariable: 'GHCR_USER',
                    passwordVariable: 'GHCR_TOKEN'
                )]) {
                    sh """
                        echo "\$GHCR_TOKEN" | docker login ghcr.io -u "\$GHCR_USER" --password-stdin

                        docker push ${IMAGE_BASE}/api:${GIT_SHA}
                        docker push ${IMAGE_BASE}/api:latest
                        docker push ${IMAGE_BASE}/worker:${GIT_SHA}
                        docker push ${IMAGE_BASE}/worker:latest
                        docker push ${IMAGE_BASE}/web:${GIT_SHA}
                        docker push ${IMAGE_BASE}/web:latest
                    """
                }
            }
        }

        // ───────────────────── CD ─────────────────────

        stage('Deploy App') {
            when {
                branch 'main'
            }
            steps {
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh """
                        echo "Updating image tags in kustomization..."
                        cd deploy/k8s/overlays/homelab

                        kustomize edit set image \
                            the-ranch=${IMAGE_BASE}/api:${GIT_SHA} \
                            the-ranch-worker=${IMAGE_BASE}/worker:${GIT_SHA} \
                            the-ranch-web=${IMAGE_BASE}/web:${GIT_SHA}

                        echo "Applying manifests..."
                        kustomize build . | kubectl apply -f -

                        echo "Waiting for rollouts..."
                        kubectl rollout status deployment/filesync-api --timeout=120s
                        kubectl rollout status deployment/filesync-worker --timeout=120s
                        kubectl rollout status deployment/filesync-web --timeout=120s

                        echo "Deploy complete."
                    """
                }
            }
        }

        stage('Deploy Infrastructure (Helm)') {
            when {
                branch 'main'
            }
            steps {
                withCredentials([file(credentialsId: 'kubeconfig', variable: 'KUBECONFIG')]) {
                    sh '''
                        # Add Helm repos
                        helm repo add bitnami https://charts.bitnami.com/bitnami || true
                        helm repo add minio https://charts.min.io/ || true
                        helm repo add tailscale https://pkgs.tailscale.com/helmcharts || true
                        helm repo update

                        # PostgreSQL
                        helm upgrade --install postgres bitnami/postgresql \
                            --namespace database --create-namespace \
                            --values helm/values/postgres.yaml \
                            --wait --timeout 5m

                        # Redis
                        helm upgrade --install redis bitnami/redis \
                            --namespace cache --create-namespace \
                            --values helm/values/redis.yaml \
                            --wait --timeout 5m

                        # MinIO
                        helm upgrade --install minio minio/minio \
                            --namespace storage --create-namespace \
                            --values helm/values/minio.yaml \
                            --wait --timeout 5m

                        # Tailscale Operator
                        helm upgrade --install tailscale-operator tailscale/tailscale-operator \
                            --namespace tailscale-system --create-namespace \
                            --set oauthSecretVolume.secret.secretName=tailscale-operator-oauth \
                            --set "operatorConfig.defaultTags={tag:k8s-operator}" \
                            --wait --timeout 5m
                    '''
                }
            }
        }
    }

    post {
        always {
            sh 'docker logout ghcr.io || true'
            cleanWs()
        }
        failure {
            echo "Pipeline failed — check the logs above for details."
        }
        success {
            echo "Pipeline completed successfully. Images: ${IMAGE_BASE}/*:${GIT_SHA}"
        }
    }
}
