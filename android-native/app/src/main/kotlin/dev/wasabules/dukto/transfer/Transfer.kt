package dev.wasabules.dukto.transfer

/**
 * TCP transfer: server (Receiver) and client (Sender).
 *
 * Reference: wails/internal/transfer/{server,receive,send}.go
 *
 * Streaming session format (see docs/PROTOCOL.md §4):
 *   [SessionHeader] then for each element [ElementHeader] [bytes...]
 *
 * Notes specific to Android:
 *  - Long-running transfers must run in a foreground service (manifest
 *    declares android.permission.FOREGROUND_SERVICE_DATA_SYNC).
 *  - Use Storage Access Framework for both reading user-selected files
 *    (ACTION_OPEN_DOCUMENT / ACTION_OPEN_DOCUMENT_TREE) and for writing
 *    received files into a destination tree.
 *
 * TODO: Server (ServerSocket on port 4644), Receiver (parse session
 * header, stream into SAF DocumentFile outputs), Sender (write
 * SessionHeader + ElementHeader frames). Plug a policy hook (mirror of
 * the Go AllowSession callback) so a future security UI can gate
 * accepts.
 */
class Server

class Sender
