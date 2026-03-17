#!/bin/bash

# AI Provider Deployment Script
# This script handles deployment of the AI Provider application

set -e  # Exit on error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DOCKER_DIR="${PROJECT_ROOT}/deployments/docker"
KUBE_DIR="${PROJECT_ROOT}/deployments/kubernetes"
COMPOSE_FILE="${DOCKER_DIR}/docker-compose.yml"
ENV_FILE="${PROJECT_ROOT}/.env"

# Default values
ENVIRONMENT="development"
ACTION="deploy"
BUILD=false
PUSH=false
VERBOSE=false
SERVICES=""

# Docker image configuration
IMAGE_NAME="ai-provider"
IMAGE_TAG="latest"
REGISTRY=""

# Kubernetes configuration
KUBE_CONTEXT=""
KUBE_NAMESPACE="default"

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Print usage
usage() {
    cat << EOF
AI Provider Deployment Script

Usage: $0 [OPTIONS] ACTION

Actions:
    deploy          Deploy the application (default)
    stop            Stop the application
    restart         Restart the application
    status          Check application status
    logs            View application logs
    clean           Clean up containers and volumes
    build           Build Docker images
    push            Push Docker images to registry

Options:
    -e, --environment ENV    Environment (development, staging, production) [default: development]
    -t, --tag TAG           Docker image tag [default: latest]
    -r, --registry REG      Docker registry URL
    -s, --services SVC      Specific services to deploy (comma-separated)
    -b, --build             Build images before deployment
    -p, --push              Push images to registry
    -k, --kubernetes        Deploy to Kubernetes instead of Docker Compose
    -n, --namespace NS      Kubernetes namespace [default: default]
    -c, --context CTX       Kubernetes context
    -v, --verbose           Verbose output
    -h, --help              Show this help message

Examples:
    # Deploy to development environment
    $0 deploy -e development

    # Deploy to production with custom tag
    $0 deploy -e production -t v1.0.0

    # Build and deploy to staging
    $0 deploy -e staging -b

    # Deploy specific services
    $0 deploy -s "ai-provider,postgres"

    # Deploy to Kubernetes
    $0 deploy -e production -k -n ai-provider

    # Stop all services
    $0 stop

    # View logs
    $0 logs -s ai-provider

EOF
    exit 1
}

# Parse command line arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--environment)
                ENVIRONMENT="$2"
                shift 2
                ;;
            -t|--tag)
                IMAGE_TAG="$2"
                shift 2
                ;;
            -r|--registry)
                REGISTRY="$2"
                shift 2
                ;;
            -s|--services)
                SERVICES="$2"
                shift 2
                ;;
            -b|--build)
                BUILD=true
                shift
                ;;
            -p|--push)
                PUSH=true
                shift
                ;;
            -k|--kubernetes)
                USE_KUBERNETES=true
                shift
                ;;
            -n|--namespace)
                KUBE_NAMESPACE="$2"
                shift 2
                ;;
            -c|--context)
                KUBE_CONTEXT="$2"
                shift 2
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            deploy|stop|restart|status|logs|clean|build|push)
                ACTION="$1"
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                usage
                ;;
        esac
    done
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        log_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi

    # Check kubectl if using Kubernetes
    if [ "$USE_KUBERNETES" = true ]; then
        if ! command -v kubectl &> /dev/null; then
            log_error "kubectl is not installed. Please install kubectl first."
            exit 1
        fi
    fi

    # Check if running as root (not recommended)
    if [ "$EUID" -eq 0 ]; then
        log_warning "Running as root is not recommended. Please run as a regular user."
    fi

    log_success "Prerequisites check passed"
}

# Load environment variables
load_env() {
    if [ -f "$ENV_FILE" ]; then
        log_info "Loading environment variables from $ENV_FILE"
        export $(cat "$ENV_FILE" | grep -v '^#' | xargs)
    else
        log_warning "Environment file $ENV_FILE not found. Using defaults."
    fi

    # Set environment-specific variables
    export COMPOSE_PROJECT_NAME="ai-provider-${ENVIRONMENT}"
    export IMAGE_TAG="$IMAGE_TAG"

    if [ -n "$REGISTRY" ]; then
        export IMAGE_NAME="${REGISTRY}/${IMAGE_NAME}"
    fi
}

# Build Docker images
build_images() {
    log_info "Building Docker images..."

    cd "$PROJECT_ROOT"

    # Build main application image
    log_info "Building ${IMAGE_NAME}:${IMAGE_TAG}"
    docker build \
        -t "${IMAGE_NAME}:${IMAGE_TAG}" \
        -t "${IMAGE_NAME}:latest" \
        -f "${DOCKER_DIR}/Dockerfile" \
        --build-arg BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ') \
        --build-arg VERSION="${IMAGE_TAG}" \
        --build-arg VCS_REF=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown") \
        .

    if [ $? -eq 0 ]; then
        log_success "Successfully built ${IMAGE_NAME}:${IMAGE_TAG}"
    else
        log_error "Failed to build ${IMAGE_NAME}:${IMAGE_TAG}"
        exit 1
    fi

    cd "$SCRIPT_DIR"
}

# Push Docker images to registry
push_images() {
    if [ -z "$REGISTRY" ]; then
        log_error "Registry URL is required for pushing images. Use -r option."
        exit 1
    fi

    log_info "Pushing Docker images to registry..."

    # Push image with specific tag
    docker push "${IMAGE_NAME}:${IMAGE_TAG}"

    if [ $? -eq 0 ]; then
        log_success "Successfully pushed ${IMAGE_NAME}:${IMAGE_TAG}"
    else
        log_error "Failed to push ${IMAGE_NAME}:${IMAGE_TAG}"
        exit 1
    fi

    # Also push latest tag if not already latest
    if [ "$IMAGE_TAG" != "latest" ]; then
        docker push "${IMAGE_NAME}:latest"
    fi
}

# Deploy using Docker Compose
deploy_compose() {
    log_info "Deploying with Docker Compose..."

    cd "$DOCKER_DIR"

    # Build images if requested
    if [ "$BUILD" = true ]; then
        build_images
    fi

    # Deploy services
    local compose_cmd="docker-compose -f docker-compose.yml"

    if [ -f "docker-compose.${ENVIRONMENT}.yml" ]; then
        compose_cmd="$compose_cmd -f docker-compose.${ENVIRONMENT}.yml"
        log_info "Using environment-specific compose file: docker-compose.${ENVIRONMENT}.yml"
    fi

    # Deploy specific services or all services
    if [ -n "$SERVICES" ]; then
        log_info "Deploying services: $SERVICES"
        $compose_cmd up -d $SERVICES
    else
        log_info "Deploying all services"
        $compose_cmd up -d
    fi

    if [ $? -eq 0 ]; then
        log_success "Deployment completed successfully"
        show_status
    else
        log_error "Deployment failed"
        exit 1
    fi

    cd "$SCRIPT_DIR"
}

# Deploy to Kubernetes
deploy_kubernetes() {
    log_info "Deploying to Kubernetes..."

    if [ -n "$KUBE_CONTEXT" ]; then
        kubectl config use-context "$KUBE_CONTEXT"
    fi

    # Create namespace if it doesn't exist
    kubectl create namespace "$KUBE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

    # Apply Kubernetes manifests
    local manifest_dir="${KUBE_DIR}/manifests"

    if [ -d "${manifest_dir}/${ENVIRONMENT}" ]; then
        manifest_dir="${manifest_dir}/${ENVIRONMENT}"
    fi

    log_info "Applying Kubernetes manifests from $manifest_dir"

    # Apply configurations first
    if [ -d "${manifest_dir}/configs" ]; then
        kubectl apply -f "${manifest_dir}/configs" -n "$KUBE_NAMESPACE"
    fi

    # Apply secrets
    if [ -d "${manifest_dir}/secrets" ]; then
        kubectl apply -f "${manifest_dir}/secrets" -n "$KUBE_NAMESPACE"
    fi

    # Apply deployments
    if [ -d "${manifest_dir}/deployments" ]; then
        kubectl apply -f "${manifest_dir}/deployments" -n "$KUBE_NAMESPACE"
    fi

    # Apply services
    if [ -d "${manifest_dir}/services" ]; then
        kubectl apply -f "${manifest_dir}/services" -n "$KUBE_NAMESPACE"
    fi

    # Wait for rollout
    log_info "Waiting for deployment rollout..."
    kubectl rollout status deployment/ai-provider -n "$KUBE_NAMESPACE" --timeout=300s

    if [ $? -eq 0 ]; then
        log_success "Kubernetes deployment completed successfully"
        kubectl get pods -n "$KUBE_NAMESPACE" -l app=ai-provider
    else
        log_error "Kubernetes deployment failed"
        exit 1
    fi
}

# Stop services
stop_services() {
    log_info "Stopping services..."

    if [ "$USE_KUBERNETES" = true ]; then
        kubectl delete deployment ai-provider -n "$KUBE_NAMESPACE" --ignore-not-found=true
    else
        cd "$DOCKER_DIR"
        docker-compose -f docker-compose.yml down
        cd "$SCRIPT_DIR"
    fi

    log_success "Services stopped"
}

# Restart services
restart_services() {
    log_info "Restarting services..."

    stop_services
    sleep 2

    if [ "$USE_KUBERNETES" = true ]; then
        deploy_kubernetes
    else
        deploy_compose
    fi
}

# Show status
show_status() {
    log_info "Application status:"

    if [ "$USE_KUBERNETES" = true ]; then
        kubectl get all -n "$KUBE_NAMESPACE" -l app=ai-provider
        echo ""
        kubectl get pods -n "$KUBE_NAMESPACE" -l app=ai-provider -o wide
    else
        cd "$DOCKER_DIR"
        docker-compose -f docker-compose.yml ps
        cd "$SCRIPT_DIR"
    fi
}

# View logs
view_logs() {
    cd "$DOCKER_DIR"

    local logs_cmd="docker-compose -f docker-compose.yml logs"

    if [ -n "$SERVICES" ]; then
        logs_cmd="$logs_cmd $SERVICES"
    fi

    if [ "$VERBOSE" = true ]; then
        $logs_cmd
    else
        $logs_cmd --tail=100 -f
    fi

    cd "$SCRIPT_DIR"
}

# Clean up
clean_up() {
    log_warning "This will remove all containers, volumes, and networks."
    read -p "Are you sure? (y/N) " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Cleaning up..."

        if [ "$USE_KUBERNETES" = true ]; then
            kubectl delete all -n "$KUBE_NAMESPACE" -l app=ai-provider
            kubectl delete pvc -n "$KUBE_NAMESPACE" -l app=ai-provider
        else
            cd "$DOCKER_DIR"
            docker-compose -f docker-compose.yml down -v --remove-orphans
            docker system prune -f
            cd "$SCRIPT_DIR"
        fi

        log_success "Cleanup completed"
    else
        log_info "Cleanup cancelled"
    fi
}

# Health check
health_check() {
    log_info "Performing health check..."

    local max_retries=30
    local retry_interval=5
    local retry_count=0

    while [ $retry_count -lt $max_retries ]; do
        if curl -f http://localhost:8080/health &> /dev/null; then
            log_success "Application is healthy!"
            curl -s http://localhost:8080/health | jq '.' 2>/dev/null || curl -s http://localhost:8080/health
            return 0
        fi

        retry_count=$((retry_count + 1))
        log_info "Waiting for application to be ready... (attempt $retry_count/$max_retries)"
        sleep $retry_interval
    done

    log_error "Application health check failed after $max_retries attempts"
    return 1
}

# Main function
main() {
    parse_args "$@"

    log_info "AI Provider Deployment Script"
    log_info "Environment: $ENVIRONMENT"
    log_info "Action: $ACTION"

    if [ "$VERBOSE" = true ]; then
        log_info "Verbose mode enabled"
        set -x
    fi

    check_prerequisites
    load_env

    case $ACTION in
        deploy)
            if [ "$USE_KUBERNETES" = true ]; then
                if [ "$BUILD" = true ]; then
                    build_images
                fi
                if [ "$PUSH" = true ]; then
                    push_images
                fi
                deploy_kubernetes
            else
                deploy_compose
            fi
            health_check
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_services
            health_check
            ;;
        status)
            show_status
            ;;
        logs)
            view_logs
            ;;
        clean)
            clean_up
            ;;
        build)
            build_images
            ;;
        push)
            push_images
            ;;
        *)
            log_error "Unknown action: $ACTION"
            usage
            ;;
    esac

    log_success "Operation completed: $ACTION"
}

# Run main function
main "$@"
