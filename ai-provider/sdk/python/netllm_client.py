"""
Netllm AI Provider - Python SDK Client
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

A comprehensive Python client library for the Netllm AI Provider API.
Supports synchronous and asynchronous inference, streaming (SSE),
model management, batch processing, and more.

Usage:
    from netllm_client import NetllmClient

    client = NetllmClient(base_url="http://localhost:8080", api_key="your-key")

    # Synchronous inference
    response = client.inference("model-id", prompt="Hello, world!")

    # Chat completion
    response = client.chat("model-id", messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "What is AI?"},
    ])

    # Streaming
    for chunk in client.inference_stream("model-id", prompt="Tell me a story"):
        print(chunk.delta, end="", flush=True)

    # Batch inference
    result = client.batch_inference("model-id", prompts=["Hi", "Hello", "Hey"])

    # Model management
    models = client.list_models()
    model = client.get_model("model-id")
    client.register_model(name="my-model", version="1.0", format="gguf", source_url="https://...")
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import dataclass, field
from enum import Enum
from typing import (
    Any,
    Callable,
    Dict,
    Generator,
    Iterator,
    List,
    Optional,
    Sequence,
    Union,
)

# Standard library imports for HTTP
try:
    from urllib.error import HTTPError, URLError
    from urllib.parse import urlencode, urlparse
    from urllib.request import Request, urlopen
except ImportError:
    pass

try:
    import http.client as http_client
except ImportError:
    import httplib as http_client  # type: ignore[no-redef]


# ---------------------------------------------------------------------------
# Version
# ---------------------------------------------------------------------------

__version__ = "1.0.0"
__author__ = "Netllm Contributors"


# ---------------------------------------------------------------------------
# Enums
# ---------------------------------------------------------------------------


class ModelStatus(str, Enum):
    """Possible statuses for a registered model."""

    REGISTERED = "registered"
    DOWNLOADING = "downloading"
    DOWNLOADED = "downloaded"
    VALIDATING = "validating"
    VALIDATED = "validated"
    ACTIVE = "active"
    INACTIVE = "inactive"
    ERROR = "error"
    DELETING = "deleting"


class ModelFormat(str, Enum):
    """Supported model file formats."""

    GGUF = "gguf"
    ONNX = "onnx"
    PYTORCH = "pytorch"
    SAFETENSORS = "safetensors"
    TENSORFLOW = "tensorflow"


class InferenceMode(str, Enum):
    """Inference execution modes."""

    SYNC = "sync"
    ASYNC = "async"
    STREAMING = "streaming"
    BATCH = "batch"


class BatchStatus(str, Enum):
    """Batch processing statuses."""

    PENDING = "pending"
    RUNNING = "running"
    COMPLETE = "complete"
    PARTIAL = "partial"
    FAILED = "failed"
    CANCELLED = "cancelled"


class RequestPriority(int, Enum):
    """Priority levels for inference requests."""

    LOW = 1
    NORMAL = 5
    HIGH = 10
    CRITICAL = 15


# ---------------------------------------------------------------------------
# Data Classes - Models
# ---------------------------------------------------------------------------


@dataclass
class ChatMessage:
    """A single message in a chat conversation."""

    role: str
    content: str
    name: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        d: Dict[str, Any] = {"role": self.role, "content": self.content}
        if self.name is not None:
            d["name"] = self.name
        return d


@dataclass
class ModelInfo:
    """Information about a registered model."""

    id: str
    name: str
    version: str
    format: str
    status: str
    size_bytes: int = 0
    description: Optional[str] = None
    source_url: Optional[str] = None
    checksum: Optional[str] = None
    created_at: Optional[str] = None
    updated_at: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ModelInfo":
        return cls(
            id=data.get("id", ""),
            name=data.get("name", ""),
            version=data.get("version", ""),
            format=data.get("format", ""),
            status=data.get("status", ""),
            size_bytes=data.get("size_bytes", 0),
            description=data.get("description"),
            source_url=data.get("source_url"),
            checksum=data.get("checksum"),
            created_at=data.get("created_at"),
            updated_at=data.get("updated_at"),
            metadata=data.get("metadata"),
        )


@dataclass
class ModelListResult:
    """Result of listing models with pagination."""

    data: List[ModelInfo]
    total: int
    page: int
    per_page: int
    total_pages: int

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ModelListResult":
        models = [ModelInfo.from_dict(m) for m in data.get("data", [])]
        return cls(
            data=models,
            total=data.get("total", 0),
            page=data.get("page", 1),
            per_page=data.get("per_page", 20),
            total_pages=data.get("total_pages", 1),
        )


# ---------------------------------------------------------------------------
# Data Classes - Inference
# ---------------------------------------------------------------------------


@dataclass
class TokenProbability:
    """Probability information for a single token."""

    token: str
    probability: float
    log_prob: float


@dataclass
class InferenceResponse:
    """Response from an inference request."""

    id: str
    request_id: str
    model_id: str
    content: str
    finish_reason: str
    input_tokens: int
    output_tokens: int
    total_tokens: int
    latency_ms: float
    time_to_first_token_ms: float
    tokens_per_second: float
    instance_id: Optional[str] = None
    probabilities: Optional[List[TokenProbability]] = None
    metadata: Optional[Dict[str, Any]] = None
    created_at: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "InferenceResponse":
        probs = None
        if "probabilities" in data and data["probabilities"]:
            probs = [
                TokenProbability(
                    token=p.get("token", ""),
                    probability=p.get("probability", 0.0),
                    log_prob=p.get("log_prob", 0.0),
                )
                for p in data["probabilities"]
            ]
        latency = data.get("latency", 0)
        if isinstance(latency, (int, float)):
            latency_ms = latency / 1_000_000 if latency > 1000 else latency
        else:
            latency_ms = 0.0

        ttft = data.get("time_to_first_token", 0)
        if isinstance(ttft, (int, float)):
            ttft_ms = ttft / 1_000_000 if ttft > 1000 else ttft
        else:
            ttft_ms = 0.0

        return cls(
            id=data.get("id", ""),
            request_id=data.get("request_id", ""),
            model_id=data.get("model_id", ""),
            content=data.get("content", ""),
            finish_reason=data.get("finish_reason", ""),
            input_tokens=data.get("input_tokens", 0),
            output_tokens=data.get("output_tokens", 0),
            total_tokens=data.get("total_tokens", 0),
            latency_ms=latency_ms,
            time_to_first_token_ms=ttft_ms,
            tokens_per_second=data.get("tokens_per_second", 0.0),
            instance_id=data.get("instance_id"),
            probabilities=probs,
            metadata=data.get("metadata"),
            created_at=data.get("created_at"),
        )


@dataclass
class StreamChunk:
    """A single chunk in a streaming response."""

    id: str
    request_id: str
    model_id: str
    content: str
    delta: str
    output_tokens: int
    input_tokens: int
    finish_reason: Optional[str] = None
    instance_id: Optional[str] = None
    latency_ms: float = 0.0

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "StreamChunk":
        latency = data.get("latency", 0)
        if isinstance(latency, (int, float)):
            latency_ms = latency / 1_000_000 if latency > 1000 else latency
        else:
            latency_ms = 0.0
        return cls(
            id=data.get("id", ""),
            request_id=data.get("request_id", ""),
            model_id=data.get("model_id", ""),
            content=data.get("content", ""),
            delta=data.get("delta", ""),
            output_tokens=data.get("output_tokens", 0),
            input_tokens=data.get("input_tokens", 0),
            finish_reason=data.get("finish_reason"),
            instance_id=data.get("instance_id"),
            latency_ms=latency_ms,
        )


@dataclass
class AsyncInferenceSubmission:
    """Response returned when submitting an async inference request."""

    id: str
    model_id: str
    status: str
    created_at: str

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "AsyncInferenceSubmission":
        return cls(
            id=data.get("id", ""),
            model_id=data.get("model_id", ""),
            status=data.get("status", "processing"),
            created_at=data.get("created_at", ""),
        )


@dataclass
class RequestStatus:
    """Status of an async inference request."""

    request_id: str
    model_id: str
    status: str
    error: Optional[str] = None
    created_at: Optional[str] = None
    updated_at: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "RequestStatus":
        return cls(
            request_id=data.get("request_id", data.get("RequestID", "")),
            model_id=data.get("model_id", data.get("ModelID", "")),
            status=data.get("status", data.get("Status", "")),
            error=data.get("error", data.get("Error")),
            created_at=data.get("created_at", data.get("CreatedAt")),
            updated_at=data.get("updated_at", data.get("UpdatedAt")),
        )


@dataclass
class BatchResult:
    """Result of a single item in a batch inference request."""

    index: int
    request_id: str
    success: bool
    response: Optional[InferenceResponse] = None
    error: Optional[str] = None
    duration_ms: float = 0.0

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "BatchResult":
        response = None
        if data.get("response"):
            response = InferenceResponse.from_dict(data["response"])
        return cls(
            index=data.get("index", 0),
            request_id=data.get("request_id", ""),
            success=data.get("success", False),
            response=response,
            error=data.get("error", {}).get("message")
            if isinstance(data.get("error"), dict)
            else data.get("error"),
            duration_ms=data.get("duration_ms", 0.0),
        )


@dataclass
class BatchResponse:
    """Response from a batch inference request."""

    id: str
    batch_id: str
    model_id: str
    status: BatchStatus
    results: List[BatchResult]
    total: int
    succeeded: int
    failed: int
    duration_ms: float = 0.0
    created_at: Optional[str] = None
    completed_at: Optional[str] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "BatchResponse":
        results = [BatchResult.from_dict(r) for r in data.get("results", [])]
        duration = data.get("duration", 0)
        if isinstance(duration, (int, float)):
            duration_ms = duration / 1_000_000 if duration > 1000 else duration
        else:
            duration_ms = 0.0
        return cls(
            id=data.get("id", ""),
            batch_id=data.get("batch_id", ""),
            model_id=data.get("model_id", ""),
            status=BatchStatus(data.get("status", "complete")),
            results=results,
            total=data.get("total", 0),
            succeeded=data.get("succeeded", 0),
            failed=data.get("failed", 0),
            duration_ms=duration_ms,
            created_at=data.get("created_at"),
            completed_at=data.get("completed_at"),
        )


# ---------------------------------------------------------------------------
# Data Classes - System
# ---------------------------------------------------------------------------


@dataclass
class HealthResponse:
    """Response from the health check endpoint."""

    status: str
    timestamp: str
    uptime: str
    version: str
    checks: Optional[Dict[str, Any]] = None

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "HealthResponse":
        return cls(
            status=data.get("status", ""),
            timestamp=data.get("timestamp", ""),
            uptime=data.get("uptime", ""),
            version=data.get("version", ""),
            checks=data.get("checks"),
        )


@dataclass
class VersionResponse:
    """Response from the version endpoint."""

    version: str
    build_time: str
    git_commit: str
    go_version: str = ""
    os: str = ""
    arch: str = ""

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "VersionResponse":
        return cls(
            version=data.get("version", ""),
            build_time=data.get("build_time", ""),
            git_commit=data.get("git_commit", ""),
            go_version=data.get("go_version", ""),
            os=data.get("os", ""),
            arch=data.get("arch", ""),
        )


@dataclass
class ExecutorStats:
    """Statistics from the inference executor."""

    total_requests: int = 0
    active_requests: int = 0
    queued_requests: int = 0
    completed_requests: int = 0
    failed_requests: int = 0
    avg_latency_ms: float = 0.0
    total_input_tokens: int = 0
    total_output_tokens: int = 0
    requests_per_second: float = 0.0

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ExecutorStats":
        avg_lat = data.get("AvgLatency", data.get("avg_latency", 0))
        if isinstance(avg_lat, (int, float)):
            avg_lat_ms = avg_lat / 1_000_000 if avg_lat > 1000 else avg_lat
        else:
            avg_lat_ms = 0.0
        return cls(
            total_requests=data.get("TotalRequests", data.get("total_requests", 0)),
            active_requests=data.get("ActiveRequests", data.get("active_requests", 0)),
            queued_requests=data.get("QueuedRequests", data.get("queued_requests", 0)),
            completed_requests=data.get(
                "CompletedRequests", data.get("completed_requests", 0)
            ),
            failed_requests=data.get("FailedRequests", data.get("failed_requests", 0)),
            avg_latency_ms=avg_lat_ms,
            total_input_tokens=data.get(
                "TotalInputTokens", data.get("total_input_tokens", 0)
            ),
            total_output_tokens=data.get(
                "TotalOutputTokens", data.get("total_output_tokens", 0)
            ),
            requests_per_second=data.get(
                "RequestsPerSecond", data.get("requests_per_second", 0.0)
            ),
        )


# ---------------------------------------------------------------------------
# Exceptions
# ---------------------------------------------------------------------------


class NetllmError(Exception):
    """Base exception for all Netllm SDK errors."""

    def __init__(
        self,
        message: str,
        status_code: Optional[int] = None,
        error_type: Optional[str] = None,
    ):
        super().__init__(message)
        self.message = message
        self.status_code = status_code
        self.error_type = error_type

    def __repr__(self) -> str:
        return f"NetllmError(message={self.message!r}, status_code={self.status_code}, error_type={self.error_type!r})"


class AuthenticationError(NetllmError):
    """Raised when authentication fails (401)."""

    pass


class AuthorizationError(NetllmError):
    """Raised when the user lacks permission (403)."""

    pass


class NotFoundError(NetllmError):
    """Raised when a resource is not found (404)."""

    pass


class RateLimitError(NetllmError):
    """Raised when the rate limit is exceeded (429)."""

    pass


class InferenceError(NetllmError):
    """Raised when inference execution fails."""

    pass


class ValidationError(NetllmError):
    """Raised when request validation fails (400)."""

    pass


class ServerError(NetllmError):
    """Raised when the server returns a 5xx error."""

    pass


class ConnectionError(NetllmError):
    """Raised when the connection to the server fails."""

    pass


class TimeoutError(NetllmError):
    """Raised when a request times out."""

    pass


# ---------------------------------------------------------------------------
# HTTP Transport (stdlib-only, no external dependencies)
# ---------------------------------------------------------------------------


class _HTTPTransport:
    """Low-level HTTP transport using only the Python standard library."""

    def __init__(
        self,
        base_url: str,
        api_key: Optional[str] = None,
        jwt_token: Optional[str] = None,
        timeout: float = 60.0,
        user_agent: Optional[str] = None,
        headers: Optional[Dict[str, str]] = None,
    ):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.jwt_token = jwt_token
        self.timeout = timeout
        self.user_agent = user_agent or f"netllm-python/{__version__}"
        self._extra_headers = headers or {}

    def _build_headers(self, content_type: Optional[str] = None) -> Dict[str, str]:
        hdrs: Dict[str, str] = {
            "User-Agent": self.user_agent,
            "Accept": "application/json",
        }
        hdrs.update(self._extra_headers)
        if self.api_key:
            hdrs["X-API-Key"] = self.api_key
        if self.jwt_token:
            hdrs["Authorization"] = f"Bearer {self.jwt_token}"
        if content_type:
            hdrs["Content-Type"] = content_type
        return hdrs

    def request(
        self,
        method: str,
        path: str,
        body: Optional[Dict[str, Any]] = None,
        params: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> Dict[str, Any]:
        url = f"{self.base_url}{path}"
        if params:
            filtered = {k: v for k, v in params.items() if v is not None}
            if filtered:
                url += "?" + urlencode(filtered)

        headers = self._build_headers(content_type="application/json" if body else None)
        data = json.dumps(body).encode("utf-8") if body else None

        req_timeout = timeout or self.timeout
        req = Request(url, data=data, headers=headers, method=method)

        try:
            response = urlopen(req, timeout=req_timeout)
            status_code = response.getcode()
            response_body = response.read().decode("utf-8")
            return self._parse_response(status_code, response_body)

        except HTTPError as e:
            response_body = ""
            try:
                response_body = e.read().decode("utf-8")
            except Exception:
                pass
            self._handle_http_error(e.code, response_body, url)
            raise  # Should not reach here

        except URLError as e:
            raise ConnectionError(f"Failed to connect to {url}: {e.reason}")

        except Exception as e:
            if "timed out" in str(e).lower():
                raise TimeoutError(f"Request to {url} timed out after {req_timeout}s")
            raise NetllmError(f"Unexpected error: {e}")

    def request_raw(
        self,
        method: str,
        path: str,
        body: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> "tuple[int, str, dict]":
        """Returns (status_code, response_body, headers) without parsing."""
        url = f"{self.base_url}{path}"
        headers = self._build_headers(content_type="application/json" if body else None)
        data = json.dumps(body).encode("utf-8") if body else None
        req_timeout = timeout or self.timeout
        req = Request(url, data=data, headers=headers, method=method)

        try:
            response = urlopen(req, timeout=req_timeout)
            resp_headers = (
                dict(response.getheaders()) if hasattr(response, "getheaders") else {}
            )
            return response.getcode(), response.read().decode("utf-8"), resp_headers
        except HTTPError as e:
            resp_body = ""
            try:
                resp_body = e.read().decode("utf-8")
            except Exception:
                pass
            self._handle_http_error(e.code, resp_body, url)
            raise
        except URLError as e:
            raise ConnectionError(f"Failed to connect to {url}: {e.reason}")

    def request_sse(
        self,
        method: str,
        path: str,
        body: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> Generator[Dict[str, Any], None, None]:
        """Make a request and yield Server-Sent Events as dictionaries."""
        url = f"{self.base_url}{path}"
        headers = self._build_headers(content_type="application/json" if body else None)
        headers["Accept"] = "text/event-stream"
        headers["Cache-Control"] = "no-cache"
        data = json.dumps(body).encode("utf-8") if body else None
        req_timeout = timeout or self.timeout

        parsed = urlparse(url)
        if parsed.scheme == "https":
            conn = http.client.HTTPSConnection(
                parsed.hostname, parsed.port or 443, timeout=req_timeout
            )
        else:
            conn = http.client.HTTPConnection(
                parsed.hostname, parsed.port or 80, timeout=req_timeout
            )

        conn_path = parsed.path
        if parsed.query:
            conn_path += "?" + parsed.query

        try:
            conn.request(method, conn_path, body=data, headers=headers)
            response = conn.getresponse()

            if response.status != 200:
                resp_body = response.read().decode("utf-8")
                self._handle_http_error(response.status, resp_body, url)
                return

            current_event = None
            buffer = ""

            while True:
                chunk = response.read(1)
                if not chunk:
                    break
                buffer += chunk.decode("utf-8", errors="replace")

                while "\n" in buffer:
                    line, buffer = buffer.split("\n", 1)
                    line = line.rstrip("\r")

                    if line.startswith("event: "):
                        current_event = line[7:].strip()
                    elif line.startswith("data: "):
                        data_str = line[6:].strip()
                        try:
                            event_data = json.loads(data_str)
                        except json.JSONDecodeError:
                            event_data = {"raw": data_str}

                        if current_event == "error":
                            raise InferenceError(
                                event_data.get("error", {}).get(
                                    "message", str(event_data)
                                ),
                                status_code=response.status,
                            )
                        elif current_event == "done":
                            return
                        elif current_event in ("chunk", "connected", "usage"):
                            yield {"event": current_event, "data": event_data}
                        else:
                            yield {
                                "event": current_event or "message",
                                "data": event_data,
                            }

                        current_event = None
                    elif line == "":
                        current_event = None

        except http.client.HTTPException as e:
            raise ConnectionError(f"HTTP connection error: {e}")
        finally:
            conn.close()

    @staticmethod
    def _parse_response(status_code: int, body: str) -> Dict[str, Any]:
        try:
            return json.loads(body)
        except (json.JSONDecodeError, TypeError):
            return {"raw": body, "status_code": status_code}

    @staticmethod
    def _handle_http_error(status_code: int, body: str, url: str) -> None:
        try:
            data = json.loads(body)
            error_obj = data.get("error", data)
            if isinstance(error_obj, dict):
                message = error_obj.get("message", body)
                error_type = error_obj.get("type", "")
            else:
                message = str(error_obj)
                error_type = ""
        except (json.JSONDecodeError, TypeError):
            message = body or f"HTTP {status_code}"
            error_type = ""

        exc_classes = {
            400: ValidationError,
            401: AuthenticationError,
            403: AuthorizationError,
            404: NotFoundError,
            429: RateLimitError,
        }

        if 500 <= status_code < 600:
            raise ServerError(message, status_code=status_code, error_type=error_type)

        exc_class = exc_classes.get(status_code, NetllmError)
        raise exc_class(message, status_code=status_code, error_type=error_type)


# ---------------------------------------------------------------------------
# Main Client
# ---------------------------------------------------------------------------


class NetllmClient:
    """
    Main client for the Netllm AI Provider API.

    Provides methods for inference, model management, system monitoring,
    and batch processing. Uses only Python standard library (no external deps).

    Args:
        base_url: Base URL of the Netllm API server (e.g., ``http://localhost:8080``).
        api_key: API key for authentication. Can also be set via constructor.
        jwt_token: JWT Bearer token for authentication.
        timeout: Default request timeout in seconds.
        user_agent: Custom User-Agent header.
        headers: Additional HTTP headers to send with every request.

    Example::

        client = NetllmClient(
            base_url="http://localhost:8080",
            api_key="sk-netllm-...",
            timeout=120.0,
        )
    """

    def __init__(
        self,
        base_url: str = "http://localhost:8080",
        api_key: Optional[str] = None,
        jwt_token: Optional[str] = None,
        timeout: float = 120.0,
        user_agent: Optional[str] = None,
        headers: Optional[Dict[str, str]] = None,
    ):
        self._transport = _HTTPTransport(
            base_url=base_url,
            api_key=api_key,
            jwt_token=jwt_token,
            timeout=timeout,
            user_agent=user_agent,
            headers=headers,
        )

    # -----------------------------------------------------------------------
    # Properties
    # -----------------------------------------------------------------------

    @property
    def base_url(self) -> str:
        return self._transport.base_url

    @property
    def api_key(self) -> Optional[str]:
        return self._transport.api_key

    @api_key.setter
    def api_key(self, value: Optional[str]) -> None:
        self._transport.api_key = value

    @property
    def jwt_token(self) -> Optional[str]:
        return self._transport.jwt_token

    @jwt_token.setter
    def jwt_token(self, value: Optional[str]) -> None:
        self._transport.jwt_token = value

    # -----------------------------------------------------------------------
    # Health & System
    # -----------------------------------------------------------------------

    def health(self) -> HealthResponse:
        """Check the health of the API server.

        Returns:
            HealthResponse with status, uptime, and component checks.
        """
        data = self._transport.request("GET", "/health")
        return HealthResponse.from_dict(data)

    def readiness(self) -> dict:
        """Check if the API server is ready to accept traffic.

        Returns:
            Dict with ``ready`` (bool) and optional ``reasons`` list.
        """
        return self._transport.request("GET", "/ready")

    def version(self) -> VersionResponse:
        """Get version information from the API server.

        Returns:
            VersionResponse with version, build time, and git commit.
        """
        data = self._transport.request("GET", "/version")
        return VersionResponse.from_dict(data)

    def ping(self) -> dict:
        """Lightweight liveness check.

        Returns:
            Dict with ``message`` ("pong") and ``timestamp``.
        """
        return self._transport.request("GET", "/ping")

    def system_info(self) -> dict:
        """Get detailed system information including Go runtime, memory, and build info.

        Returns:
            Dict with comprehensive system information.
        """
        return self._transport.request("GET", "/api/v1/system/info")

    def diagnostics(self) -> dict:
        """Get a full diagnostics snapshot for troubleshooting.

        Returns:
            Dict with version, system, health, and config information.
        """
        return self._transport.request("GET", "/api/v1/system/diagnostics")

    def system_metrics(self) -> dict:
        """Get system and runtime metrics.

        Returns:
            Dict with memory, runtime, and request metrics.
        """
        return self._transport.request("GET", "/api/v1/system/metrics")

    # -----------------------------------------------------------------------
    # Inference
    # -----------------------------------------------------------------------

    def inference(
        self,
        model_id: str,
        prompt: Optional[str] = None,
        messages: Optional[Sequence[Union[ChatMessage, Dict[str, str]]]] = None,
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        user: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> InferenceResponse:
        """Run synchronous inference on the specified model.

        Provide either ``prompt`` (string) or ``messages`` (chat format).

        Args:
            model_id: ID of the model to use for inference.
            prompt: Text prompt for completion.
            messages: Chat messages for conversation-style inference.
            max_tokens: Maximum number of tokens to generate.
            temperature: Sampling temperature (0.0 - 2.0).
            top_p: Nucleus sampling threshold (0.0 - 1.0).
            top_k: Top-K sampling parameter.
            stop: List of stop sequences.
            frequency_penalty: Frequency penalty (0.0 - 2.0).
            presence_penalty: Presence penalty (0.0 - 2.0).
            user: Optional user identifier for tracking.
            metadata: Optional metadata dict.
            timeout: Request-specific timeout in seconds.

        Returns:
            InferenceResponse with generated content and token usage.

        Raises:
            ValidationError: If neither prompt nor messages are provided.
            InferenceError: If inference execution fails.

        Example::

            response = client.inference(
                "llama-3-8b",
                prompt="Explain quantum computing in simple terms.",
                max_tokens=256,
                temperature=0.7,
            )
            print(response.content)
        """
        body = self._build_inference_body(
            prompt=prompt,
            messages=messages,
            max_tokens=max_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            stop=stop,
            frequency_penalty=frequency_penalty,
            presence_penalty=presence_penalty,
            user=user,
            metadata=metadata,
        )
        data = self._transport.request(
            "POST", f"/api/v1/inference/{model_id}", body=body, timeout=timeout
        )
        return InferenceResponse.from_dict(data)

    def chat(
        self,
        model_id: str,
        messages: Sequence[Union[ChatMessage, Dict[str, str]]],
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        user: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> InferenceResponse:
        """Run chat-style inference with a list of messages.

        Convenience method that wraps :meth:`inference` with a messages-first API.

        Args:
            model_id: ID of the model to use.
            messages: List of chat messages.
            max_tokens: Maximum tokens to generate.
            temperature: Sampling temperature.
            top_p: Nucleus sampling threshold.
            top_k: Top-K sampling.
            stop: Stop sequences.
            frequency_penalty: Frequency penalty.
            presence_penalty: Presence penalty.
            user: Optional user identifier.
            metadata: Optional metadata.
            timeout: Request timeout in seconds.

        Returns:
            InferenceResponse.

        Example::

            response = client.chat(
                "llama-3-8b",
                messages=[
                    {"role": "system", "content": "You are a helpful assistant."},
                    {"role": "user", "content": "What is Python?"},
                ],
                max_tokens=200,
            )
        """
        return self.inference(
            model_id=model_id,
            messages=messages,
            max_tokens=max_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            stop=stop,
            frequency_penalty=frequency_penalty,
            presence_penalty=presence_penalty,
            user=user,
            metadata=metadata,
            timeout=timeout,
        )

    def inference_async(
        self,
        model_id: str,
        prompt: Optional[str] = None,
        messages: Optional[Sequence[Union[ChatMessage, Dict[str, str]]]] = None,
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        user: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> AsyncInferenceSubmission:
        """Submit an asynchronous inference request.

        Returns immediately with a request ID. Use :meth:`get_inference_status`
        to poll for completion.

        Args:
            model_id: ID of the model.
            prompt: Text prompt.
            messages: Chat messages.
            max_tokens: Max tokens to generate.
            temperature: Sampling temperature.
            top_p: Nucleus sampling.
            top_k: Top-K sampling.
            stop: Stop sequences.
            frequency_penalty: Frequency penalty.
            presence_penalty: Presence penalty.
            user: User identifier.
            metadata: Metadata dict.

        Returns:
            AsyncInferenceSubmission with request ID and status.

        Example::

            submission = client.inference_async("llama-3-8b", prompt="Write a poem")
            # Poll for result
            while True:
                status = client.get_inference_status(submission.id)
                if status.status in ("completed", "failed"):
                    break
                time.sleep(1)
        """
        body = self._build_inference_body(
            prompt=prompt,
            messages=messages,
            max_tokens=max_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            stop=stop,
            frequency_penalty=frequency_penalty,
            presence_penalty=presence_penalty,
            user=user,
            metadata=metadata,
        )
        data = self._transport.request(
            "POST", f"/api/v1/inference/{model_id}/async", body=body
        )
        return AsyncInferenceSubmission.from_dict(data)

    def get_inference_status(self, request_id: str) -> RequestStatus:
        """Get the status of an async inference request.

        Args:
            request_id: The ID returned by :meth:`inference_async`.

        Returns:
            RequestStatus with current state and optional error.
        """
        data = self._transport.request(
            "GET", f"/api/v1/inference/requests/{request_id}"
        )
        return RequestStatus.from_dict(data)

    def cancel_inference(self, request_id: str) -> dict:
        """Cancel a pending or running inference request.

        Args:
            request_id: The request ID to cancel.

        Returns:
            Dict confirming cancellation.
        """
        return self._transport.request(
            "DELETE", f"/api/v1/inference/requests/{request_id}"
        )

    def inference_stream(
        self,
        model_id: str,
        prompt: Optional[str] = None,
        messages: Optional[Sequence[Union[ChatMessage, Dict[str, str]]]] = None,
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        user: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
        on_chunk: Optional[Callable[[StreamChunk], None]] = None,
    ) -> Generator[StreamChunk, None, None]:
        """Run streaming inference using Server-Sent Events (SSE).

        Yields :class:`StreamChunk` objects as they arrive from the server.

        Args:
            model_id: ID of the model.
            prompt: Text prompt.
            messages: Chat messages.
            max_tokens: Max tokens.
            temperature: Sampling temperature.
            top_p: Nucleus sampling.
            top_k: Top-K sampling.
            stop: Stop sequences.
            frequency_penalty: Frequency penalty.
            presence_penalty: Presence penalty.
            user: User identifier.
            metadata: Metadata dict.
            timeout: Stream timeout.
            on_chunk: Optional callback invoked for each chunk.

        Yields:
            StreamChunk objects containing incremental content.

        Example::

            for chunk in client.inference_stream("llama-3-8b", prompt="Tell me a story"):
                print(chunk.delta, end="", flush=True)
            print()  # newline after streaming completes
        """
        body = self._build_inference_body(
            prompt=prompt,
            messages=messages,
            max_tokens=max_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            stop=stop,
            frequency_penalty=frequency_penalty,
            presence_penalty=presence_penalty,
            user=user,
            metadata=metadata,
        )

        for event in self._transport.request_sse(
            "POST", f"/api/v1/inference/{model_id}/stream", body=body, timeout=timeout
        ):
            event_name = event.get("event", "")
            event_data = event.get("data", {})

            if event_name == "chunk":
                chunk = StreamChunk.from_dict(event_data)
                if on_chunk:
                    on_chunk(chunk)
                yield chunk
            elif event_name == "usage":
                # Usage statistics event; skip in generator but useful for debugging
                pass

    def batch_inference(
        self,
        model_id: str,
        prompts: Optional[List[str]] = None,
        message_sets: Optional[List[List[Union[ChatMessage, Dict[str, str]]]]] = None,
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        priority: int = 5,
        metadata: Optional[Dict[str, Any]] = None,
        timeout: Optional[float] = None,
    ) -> BatchResponse:
        """Submit a batch of inference requests for parallel processing.

        Provide either ``prompts`` (list of strings) or ``message_sets``
        (list of message lists).

        Args:
            model_id: ID of the model.
            prompts: List of text prompts.
            message_sets: List of message lists (one per request).
            max_tokens: Max tokens per request.
            temperature: Sampling temperature.
            top_p: Nucleus sampling.
            top_k: Top-K sampling.
            stop: Stop sequences.
            frequency_penalty: Frequency penalty.
            presence_penalty: Presence penalty.
            priority: Batch priority (1=low, 5=normal, 10=high, 15=critical).
            metadata: Optional metadata.
            timeout: Request timeout.

        Returns:
            BatchResponse with results for each input.

        Example::

            result = client.batch_inference(
                "llama-3-8b",
                prompts=["What is AI?", "What is ML?", "What is DL?"],
            )
            for r in result.results:
                if r.success:
                    print(r.response.content)
        """
        requests_list = []

        if prompts:
            for p in prompts:
                requests_list.append(
                    self._build_inference_body(
                        prompt=p,
                        max_tokens=max_tokens,
                        temperature=temperature,
                        top_p=top_p,
                        top_k=top_k,
                        stop=stop,
                        frequency_penalty=frequency_penalty,
                        presence_penalty=presence_penalty,
                    )
                )
        elif message_sets:
            for msgs in message_sets:
                requests_list.append(
                    self._build_inference_body(
                        messages=msgs,
                        max_tokens=max_tokens,
                        temperature=temperature,
                        top_p=top_p,
                        top_k=top_k,
                        stop=stop,
                        frequency_penalty=frequency_penalty,
                        presence_penalty=presence_penalty,
                    )
                )
        else:
            raise ValidationError("Either prompts or message_sets must be provided")

        body: Dict[str, Any] = {
            "model": model_id,
            "requests": requests_list,
            "priority": priority,
        }
        if metadata:
            body["metadata"] = metadata

        data = self._transport.request(
            "POST", "/api/v1/inference/batch", body=body, timeout=timeout
        )
        return BatchResponse.from_dict(data)

    def get_inference_stats(self) -> ExecutorStats:
        """Get inference executor statistics.

        Returns:
            ExecutorStats with throughput, latency, and queue metrics.
        """
        data = self._transport.request("GET", "/api/v1/inference/stats")
        return ExecutorStats.from_dict(data)

    def list_active_requests(self) -> List[RequestStatus]:
        """List all currently active inference requests.

        Returns:
            List of RequestStatus objects.
        """
        data = self._transport.request("GET", "/api/v1/inference/requests")
        items = data.get("data", [])
        if isinstance(items, list):
            return [RequestStatus.from_dict(r) for r in items]
        return []

    # -----------------------------------------------------------------------
    # Model Management
    # -----------------------------------------------------------------------

    def list_models(
        self,
        page: int = 1,
        per_page: int = 20,
        status: Optional[str] = None,
        format: Optional[str] = None,
        search: Optional[str] = None,
        sort_by: str = "created_at",
        sort_order: str = "DESC",
    ) -> ModelListResult:
        """List registered models with filtering and pagination.

        Args:
            page: Page number (1-indexed).
            per_page: Items per page (max 100).
            status: Filter by model status.
            format: Filter by model format.
            search: Search term for name/description.
            sort_by: Sort field.
            sort_order: ``"ASC"`` or ``"DESC"``.

        Returns:
            ModelListResult with paginated model list.
        """
        params: Dict[str, Any] = {
            "page": page,
            "per_page": per_page,
            "sort_by": sort_by,
            "sort_order": sort_order,
        }
        if status:
            params["status"] = status
        if format:
            params["format"] = format
        if search:
            params["search"] = search

        data = self._transport.request("GET", "/api/v1/models", params=params)
        return ModelListResult.from_dict(data)

    def get_model(self, model_id: str) -> ModelInfo:
        """Get details of a specific model.

        Args:
            model_id: The model ID.

        Returns:
            ModelInfo with model details.
        """
        data = self._transport.request("GET", f"/api/v1/models/{model_id}")
        return ModelInfo.from_dict(data)

    def register_model(
        self,
        name: str,
        version: str,
        format: str,
        source_url: Optional[str] = None,
        description: Optional[str] = None,
        checksum: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> ModelInfo:
        """Register a new model in the system.

        Args:
            name: Model name.
            version: Semantic version string.
            format: Model format (gguf, onnx, pytorch, safetensors).
            source_url: URL to download the model from.
            description: Human-readable description.
            checksum: Expected checksum for validation.
            metadata: Additional metadata.

        Returns:
            ModelInfo for the newly registered model.

        Example::

            model = client.register_model(
                name="llama-3-8b-instruct",
                version="1.0.0",
                format="gguf",
                source_url="https://huggingface.co/.../model.gguf",
                description="Llama 3 8B Instruct",
            )
        """
        body: Dict[str, Any] = {
            "name": name,
            "version": version,
            "format": format,
        }
        if source_url:
            body["source_url"] = source_url
        if description:
            body["description"] = description
        if checksum:
            body["checksum"] = checksum
        if metadata:
            body["metadata"] = metadata

        data = self._transport.request("POST", "/api/v1/models", body=body)
        return ModelInfo.from_dict(data)

    def update_model(
        self,
        model_id: str,
        description: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
        config: Optional[Dict[str, Any]] = None,
    ) -> ModelInfo:
        """Update a model's configuration.

        Args:
            model_id: The model ID.
            description: Updated description.
            metadata: Updated metadata.
            config: Updated model config.

        Returns:
            ModelInfo with updated details.
        """
        body: Dict[str, Any] = {}
        if description is not None:
            body["description"] = description
        if metadata is not None:
            body["metadata"] = metadata
        if config is not None:
            body["config"] = config

        data = self._transport.request("PUT", f"/api/v1/models/{model_id}", body=body)
        return ModelInfo.from_dict(data)

    def delete_model(self, model_id: str, force: bool = False) -> None:
        """Delete a model from the system.

        Args:
            model_id: The model ID.
            force: Force deletion even if model is active.
        """
        params = {"force": "true"} if force else None
        self._transport.request("DELETE", f"/api/v1/models/{model_id}", params=params)

    def download_model(self, model_id: str) -> dict:
        """Start downloading a model's files from its source URL.

        Args:
            model_id: The model ID.

        Returns:
            Dict confirming download started.
        """
        return self._transport.request("POST", f"/api/v1/models/{model_id}/download")

    def get_download_progress(self, model_id: str) -> dict:
        """Get the download progress for a model.

        Args:
            model_id: The model ID.

        Returns:
            Dict with progress details (percentage, bytes downloaded, etc.).
        """
        return self._transport.request("GET", f"/api/v1/models/{model_id}/download")

    def cancel_download(self, model_id: str) -> dict:
        """Cancel a model download in progress.

        Args:
            model_id: The model ID.

        Returns:
            Dict confirming cancellation.
        """
        return self._transport.request("DELETE", f"/api/v1/models/{model_id}/download")

    def validate_model(self, model_id: str) -> dict:
        """Validate a model's integrity and configuration.

        Args:
            model_id: The model ID.

        Returns:
            Dict with validation result.
        """
        return self._transport.request("POST", f"/api/v1/models/{model_id}/validate")

    def activate_model(self, model_id: str) -> dict:
        """Activate a model for inference.

        Args:
            model_id: The model ID.

        Returns:
            Dict confirming activation.
        """
        return self._transport.request("POST", f"/api/v1/models/{model_id}/activate")

    def deactivate_model(self, model_id: str) -> dict:
        """Deactivate a model (stop serving inference).

        Args:
            model_id: The model ID.

        Returns:
            Dict confirming deactivation.
        """
        return self._transport.request("POST", f"/api/v1/models/{model_id}/deactivate")

    def get_model_config(self, model_id: str) -> dict:
        """Get a model's runtime configuration.

        Args:
            model_id: The model ID.

        Returns:
            Dict with model configuration.
        """
        return self._transport.request("GET", f"/api/v1/models/{model_id}/config")

    def update_model_config(self, model_id: str, config: Dict[str, Any]) -> dict:
        """Update a model's runtime configuration.

        Args:
            model_id: The model ID.
            config: New configuration dict.

        Returns:
            Dict with updated configuration.
        """
        return self._transport.request(
            "PUT", f"/api/v1/models/{model_id}/config", body=config
        )

    def list_model_versions(self, model_id: str) -> dict:
        """List all versions of a model.

        Args:
            model_id: The model ID.

        Returns:
            Dict with list of versions.
        """
        return self._transport.request("GET", f"/api/v1/models/{model_id}/versions")

    def get_model_stats(self) -> dict:
        """Get aggregate statistics for all models.

        Returns:
            Dict with model statistics.
        """
        return self._transport.request("GET", "/api/v1/models/stats")

    def search_models(self, query: str) -> List[ModelInfo]:
        """Search models by name or description.

        Args:
            query: Search query string.

        Returns:
            List of matching ModelInfo objects.
        """
        data = self._transport.request(
            "GET", "/api/v1/models/search", params={"q": query}
        )
        items = data.get("data", [])
        return [ModelInfo.from_dict(m) for m in items]

    # -----------------------------------------------------------------------
    # Convenience methods
    # -----------------------------------------------------------------------

    def wait_for_inference(
        self,
        request_id: str,
        poll_interval: float = 1.0,
        max_wait: float = 300.0,
    ) -> RequestStatus:
        """Poll an async inference request until completion.

        Args:
            request_id: The async request ID.
            poll_interval: Seconds between polls.
            max_wait: Maximum wait time in seconds.

        Returns:
            Final RequestStatus.

        Raises:
            TimeoutError: If the request doesn't complete within max_wait.
        """
        start = time.time()
        while True:
            status = self.get_inference_status(request_id)
            if status.status in (
                "completed",
                "complete",
                "failed",
                "cancelled",
                "error",
            ):
                return status
            elapsed = time.time() - start
            if elapsed >= max_wait:
                raise TimeoutError(
                    f"Inference request {request_id} did not complete within {max_wait}s "
                    f"(current status: {status.status})"
                )
            time.sleep(poll_interval)

    def generate(
        self,
        model_id: str,
        prompt: str,
        max_tokens: int = 256,
        temperature: float = 0.7,
        **kwargs: Any,
    ) -> str:
        """Simple text generation that returns just the generated string.

        Convenience wrapper around :meth:`inference` that returns only the content.

        Args:
            model_id: Model ID.
            prompt: Text prompt.
            max_tokens: Max tokens.
            temperature: Sampling temperature.
            **kwargs: Additional inference parameters.

        Returns:
            Generated text string.
        """
        response = self.inference(
            model_id=model_id,
            prompt=prompt,
            max_tokens=max_tokens,
            temperature=temperature,
            **kwargs,
        )
        return response.content

    # -----------------------------------------------------------------------
    # Internal helpers
    # -----------------------------------------------------------------------

    @staticmethod
    def _serialize_messages(
        messages: Optional[Sequence[Union[ChatMessage, Dict[str, str]]]] = None,
    ) -> Optional[List[Dict[str, str]]]:
        if messages is None:
            return None
        result = []
        for msg in messages:
            if isinstance(msg, ChatMessage):
                result.append(msg.to_dict())
            elif isinstance(msg, dict):
                result.append(msg)
            else:
                raise ValidationError(f"Invalid message type: {type(msg)}")
        return result

    @staticmethod
    def _build_inference_body(
        prompt: Optional[str] = None,
        messages: Optional[Sequence[Union[ChatMessage, Dict[str, str]]]] = None,
        max_tokens: int = 512,
        temperature: float = 0.7,
        top_p: float = 0.9,
        top_k: int = 40,
        stop: Optional[List[str]] = None,
        frequency_penalty: float = 0.0,
        presence_penalty: float = 0.0,
        user: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        body: Dict[str, Any] = {
            "max_tokens": max_tokens,
            "temperature": temperature,
            "top_p": top_p,
            "top_k": top_k,
            "frequency_penalty": frequency_penalty,
            "presence_penalty": presence_penalty,
        }
        if prompt:
            body["prompt"] = prompt
        if messages:
            serialized = []
            for msg in messages:
                if isinstance(msg, ChatMessage):
                    serialized.append(msg.to_dict())
                elif isinstance(msg, dict):
                    serialized.append(msg)
                else:
                    raise ValidationError(f"Invalid message type: {type(msg)}")
            body["messages"] = serialized
        if stop:
            body["stop"] = stop
        if user:
            body["user"] = user
        if metadata:
            body["metadata"] = metadata
        return body


# ---------------------------------------------------------------------------
# Client Builder / Factory
# ---------------------------------------------------------------------------


class NetllmClientBuilder:
    """Builder for constructing a :class:`NetllmClient` with optional parameters.

    Example::

        client = (
            NetllmClientBuilder()
            .base_url("http://localhost:8080")
            .api_key("sk-netllm-...")
            .timeout(120.0)
            .header("X-Custom-Header", "value")
            .build()
        )
    """

    def __init__(self) -> None:
        self._base_url = "http://localhost:8080"
        self._api_key: Optional[str] = None
        self._jwt_token: Optional[str] = None
        self._timeout = 120.0
        self._user_agent: Optional[str] = None
        self._headers: Dict[str, str] = {}

    def base_url(self, url: str) -> "NetllmClientBuilder":
        self._base_url = url
        return self

    def api_key(self, key: str) -> "NetllmClientBuilder":
        self._api_key = key
        return self

    def jwt_token(self, token: str) -> "NetllmClientBuilder":
        self._jwt_token = token
        return self

    def timeout(self, seconds: float) -> "NetllmClientBuilder":
        self._timeout = seconds
        return self

    def user_agent(self, ua: str) -> "NetllmClientBuilder":
        self._user_agent = ua
        return self

    def header(self, key: str, value: str) -> "NetllmClientBuilder":
        self._headers[key] = value
        return self

    def headers(self, hdrs: Dict[str, str]) -> "NetllmClientBuilder":
        self._headers.update(hdrs)
        return self

    def build(self) -> NetllmClient:
        return NetllmClient(
            base_url=self._base_url,
            api_key=self._api_key,
            jwt_token=self._jwt_token,
            timeout=self._timeout,
            user_agent=self._user_agent,
            headers=self._headers if self._headers else None,
        )


# ---------------------------------------------------------------------------
# Package-level convenience
# ---------------------------------------------------------------------------


def create_client(
    base_url: str = "http://localhost:8080",
    api_key: Optional[str] = None,
    **kwargs: Any,
) -> NetllmClient:
    """Create a new :class:`NetllmClient` instance.

    This is a convenience function equivalent to calling the constructor directly.

    Args:
        base_url: API server base URL.
        api_key: API key for authentication.
        **kwargs: Additional arguments passed to :class:`NetllmClient`.

    Returns:
        Configured NetllmClient instance.
    """
    return NetllmClient(base_url=base_url, api_key=api_key, **kwargs)
