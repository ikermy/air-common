// package: air.v1
// file: air/v1/air.proto

import * as jspb from "google-protobuf";

export class File extends jspb.Message {
  getName(): string;
  setName(value: string): void;

  getMimeType(): string;
  setMimeType(value: string): void;

  getData(): Uint8Array | string;
  getData_asU8(): Uint8Array;
  getData_asB64(): string;
  setData(value: Uint8Array | string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): File.AsObject;
  static toObject(includeInstance: boolean, msg: File): File.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: File, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): File;
  static deserializeBinaryFromReader(message: File, reader: jspb.BinaryReader): File;
}

export namespace File {
  export type AsObject = {
    name: string,
    mimeType: string,
    data: Uint8Array | string,
  }
}

export class ModelRequest extends jspb.Message {
  hasStart(): boolean;
  clearStart(): void;
  getStart(): ModelStart | undefined;
  setStart(value?: ModelStart): void;

  hasCancel(): boolean;
  clearCancel(): void;
  getCancel(): ModelCancel | undefined;
  setCancel(value?: ModelCancel): void;

  getPayloadCase(): ModelRequest.PayloadCase;
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelRequest.AsObject;
  static toObject(includeInstance: boolean, msg: ModelRequest): ModelRequest.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelRequest;
  static deserializeBinaryFromReader(message: ModelRequest, reader: jspb.BinaryReader): ModelRequest;
}

export namespace ModelRequest {
  export type AsObject = {
    start?: ModelStart.AsObject,
    cancel?: ModelCancel.AsObject,
  }

  export enum PayloadCase {
    PAYLOAD_NOT_SET = 0,
    START = 1,
    CANCEL = 2,
  }
}

export class ModelStart extends jspb.Message {
  getRequestId(): string;
  setRequestId(value: string): void;

  getOrigin(): string;
  setOrigin(value: string): void;

  getText(): string;
  setText(value: string): void;

  getContext(): string;
  setContext(value: string): void;

  getDialogId(): number;
  setDialogId(value: number): void;

  clearFilesList(): void;
  getFilesList(): Array<File>;
  setFilesList(value: Array<File>): void;
  addFiles(value?: File, index?: number): File;

  getProtocolVersion(): string;
  setProtocolVersion(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelStart.AsObject;
  static toObject(includeInstance: boolean, msg: ModelStart): ModelStart.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelStart, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelStart;
  static deserializeBinaryFromReader(message: ModelStart, reader: jspb.BinaryReader): ModelStart;
}

export namespace ModelStart {
  export type AsObject = {
    requestId: string,
    origin: string,
    text: string,
    context: string,
    dialogId: number,
    filesList: Array<File.AsObject>,
    protocolVersion: string,
  }
}

export class ModelCancel extends jspb.Message {
  getRequestId(): string;
  setRequestId(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelCancel.AsObject;
  static toObject(includeInstance: boolean, msg: ModelCancel): ModelCancel.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelCancel, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelCancel;
  static deserializeBinaryFromReader(message: ModelCancel, reader: jspb.BinaryReader): ModelCancel;
}

export namespace ModelCancel {
  export type AsObject = {
    requestId: string,
  }
}

export class ModelEvent extends jspb.Message {
  getRequestId(): string;
  setRequestId(value: string): void;

  hasDelta(): boolean;
  clearDelta(): void;
  getDelta(): ModelDelta | undefined;
  setDelta(value?: ModelDelta): void;

  hasDone(): boolean;
  clearDone(): void;
  getDone(): ModelDone | undefined;
  setDone(value?: ModelDone): void;

  hasError(): boolean;
  clearError(): void;
  getError(): ModelError | undefined;
  setError(value?: ModelError): void;

  getEventCase(): ModelEvent.EventCase;
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelEvent.AsObject;
  static toObject(includeInstance: boolean, msg: ModelEvent): ModelEvent.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelEvent, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelEvent;
  static deserializeBinaryFromReader(message: ModelEvent, reader: jspb.BinaryReader): ModelEvent;
}

export namespace ModelEvent {
  export type AsObject = {
    requestId: string,
    delta?: ModelDelta.AsObject,
    done?: ModelDone.AsObject,
    error?: ModelError.AsObject,
  }

  export enum EventCase {
    EVENT_NOT_SET = 0,
    DELTA = 2,
    DONE = 3,
    ERROR = 4,
  }
}

export class ModelDelta extends jspb.Message {
  getText(): string;
  setText(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelDelta.AsObject;
  static toObject(includeInstance: boolean, msg: ModelDelta): ModelDelta.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelDelta, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelDelta;
  static deserializeBinaryFromReader(message: ModelDelta, reader: jspb.BinaryReader): ModelDelta;
}

export namespace ModelDelta {
  export type AsObject = {
    text: string,
  }
}

export class ModelDone extends jspb.Message {
  getMessage(): string;
  setMessage(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelDone.AsObject;
  static toObject(includeInstance: boolean, msg: ModelDone): ModelDone.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelDone, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelDone;
  static deserializeBinaryFromReader(message: ModelDone, reader: jspb.BinaryReader): ModelDone;
}

export namespace ModelDone {
  export type AsObject = {
    message: string,
  }
}

export class ModelError extends jspb.Message {
  getCode(): string;
  setCode(value: string): void;

  getMessage(): string;
  setMessage(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): ModelError.AsObject;
  static toObject(includeInstance: boolean, msg: ModelError): ModelError.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: ModelError, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): ModelError;
  static deserializeBinaryFromReader(message: ModelError, reader: jspb.BinaryReader): ModelError;
}

export namespace ModelError {
  export type AsObject = {
    code: string,
    message: string,
  }
}

export class SttRequest extends jspb.Message {
  hasStart(): boolean;
  clearStart(): void;
  getStart(): SttStart | undefined;
  setStart(value?: SttStart): void;

  hasAudio(): boolean;
  clearAudio(): void;
  getAudio(): SttAudio | undefined;
  setAudio(value?: SttAudio): void;

  hasStop(): boolean;
  clearStop(): void;
  getStop(): SttStop | undefined;
  setStop(value?: SttStop): void;

  getPayloadCase(): SttRequest.PayloadCase;
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttRequest.AsObject;
  static toObject(includeInstance: boolean, msg: SttRequest): SttRequest.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttRequest;
  static deserializeBinaryFromReader(message: SttRequest, reader: jspb.BinaryReader): SttRequest;
}

export namespace SttRequest {
  export type AsObject = {
    start?: SttStart.AsObject,
    audio?: SttAudio.AsObject,
    stop?: SttStop.AsObject,
  }

  export enum PayloadCase {
    PAYLOAD_NOT_SET = 0,
    START = 1,
    AUDIO = 2,
    STOP = 3,
  }
}

export class SttStart extends jspb.Message {
  getSessionId(): string;
  setSessionId(value: string): void;

  getOrigin(): string;
  setOrigin(value: string): void;

  getSampleRate(): number;
  setSampleRate(value: number): void;

  getChannels(): number;
  setChannels(value: number): void;

  getLanguage(): string;
  setLanguage(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttStart.AsObject;
  static toObject(includeInstance: boolean, msg: SttStart): SttStart.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttStart, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttStart;
  static deserializeBinaryFromReader(message: SttStart, reader: jspb.BinaryReader): SttStart;
}

export namespace SttStart {
  export type AsObject = {
    sessionId: string,
    origin: string,
    sampleRate: number,
    channels: number,
    language: string,
  }
}

export class SttAudio extends jspb.Message {
  getPcm16(): Uint8Array | string;
  getPcm16_asU8(): Uint8Array;
  getPcm16_asB64(): string;
  setPcm16(value: Uint8Array | string): void;

  getSequence(): number;
  setSequence(value: number): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttAudio.AsObject;
  static toObject(includeInstance: boolean, msg: SttAudio): SttAudio.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttAudio, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttAudio;
  static deserializeBinaryFromReader(message: SttAudio, reader: jspb.BinaryReader): SttAudio;
}

export namespace SttAudio {
  export type AsObject = {
    pcm16: Uint8Array | string,
    sequence: number,
  }
}

export class SttStop extends jspb.Message {
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttStop.AsObject;
  static toObject(includeInstance: boolean, msg: SttStop): SttStop.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttStop, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttStop;
  static deserializeBinaryFromReader(message: SttStop, reader: jspb.BinaryReader): SttStop;
}

export namespace SttStop {
  export type AsObject = {
  }
}

export class SttEvent extends jspb.Message {
  getSessionId(): string;
  setSessionId(value: string): void;

  hasPartial(): boolean;
  clearPartial(): void;
  getPartial(): SttPartial | undefined;
  setPartial(value?: SttPartial): void;

  hasFinal(): boolean;
  clearFinal(): void;
  getFinal(): SttFinal | undefined;
  setFinal(value?: SttFinal): void;

  hasError(): boolean;
  clearError(): void;
  getError(): SttError | undefined;
  setError(value?: SttError): void;

  getEventCase(): SttEvent.EventCase;
  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttEvent.AsObject;
  static toObject(includeInstance: boolean, msg: SttEvent): SttEvent.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttEvent, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttEvent;
  static deserializeBinaryFromReader(message: SttEvent, reader: jspb.BinaryReader): SttEvent;
}

export namespace SttEvent {
  export type AsObject = {
    sessionId: string,
    partial?: SttPartial.AsObject,
    pb_final?: SttFinal.AsObject,
    error?: SttError.AsObject,
  }

  export enum EventCase {
    EVENT_NOT_SET = 0,
    PARTIAL = 2,
    FINAL = 3,
    ERROR = 4,
  }
}

export class SttPartial extends jspb.Message {
  getText(): string;
  setText(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttPartial.AsObject;
  static toObject(includeInstance: boolean, msg: SttPartial): SttPartial.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttPartial, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttPartial;
  static deserializeBinaryFromReader(message: SttPartial, reader: jspb.BinaryReader): SttPartial;
}

export namespace SttPartial {
  export type AsObject = {
    text: string,
  }
}

export class SttFinal extends jspb.Message {
  getText(): string;
  setText(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttFinal.AsObject;
  static toObject(includeInstance: boolean, msg: SttFinal): SttFinal.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttFinal, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttFinal;
  static deserializeBinaryFromReader(message: SttFinal, reader: jspb.BinaryReader): SttFinal;
}

export namespace SttFinal {
  export type AsObject = {
    text: string,
  }
}

export class SttError extends jspb.Message {
  getCode(): string;
  setCode(value: string): void;

  getMessage(): string;
  setMessage(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): SttError.AsObject;
  static toObject(includeInstance: boolean, msg: SttError): SttError.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: SttError, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): SttError;
  static deserializeBinaryFromReader(message: SttError, reader: jspb.BinaryReader): SttError;
}

export namespace SttError {
  export type AsObject = {
    code: string,
    message: string,
  }
}

export class RelayFrame extends jspb.Message {
  getSessionId(): string;
  setSessionId(value: string): void;

  getSequence(): number;
  setSequence(value: number): void;

  getType(): string;
  setType(value: string): void;

  getPayload(): Uint8Array | string;
  getPayload_asU8(): Uint8Array;
  getPayload_asB64(): string;
  setPayload(value: Uint8Array | string): void;

  hasHello(): boolean;
  clearHello(): void;
  getHello(): AgentHello | undefined;
  setHello(value?: AgentHello): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): RelayFrame.AsObject;
  static toObject(includeInstance: boolean, msg: RelayFrame): RelayFrame.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: RelayFrame, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): RelayFrame;
  static deserializeBinaryFromReader(message: RelayFrame, reader: jspb.BinaryReader): RelayFrame;
}

export namespace RelayFrame {
  export type AsObject = {
    sessionId: string,
    sequence: number,
    type: string,
    payload: Uint8Array | string,
    hello?: AgentHello.AsObject,
  }
}

export class AgentHello extends jspb.Message {
  getDeviceId(): string;
  setDeviceId(value: string): void;

  getAgentVersion(): string;
  setAgentVersion(value: string): void;

  getProtocolVersion(): string;
  setProtocolVersion(value: string): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): AgentHello.AsObject;
  static toObject(includeInstance: boolean, msg: AgentHello): AgentHello.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: AgentHello, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): AgentHello;
  static deserializeBinaryFromReader(message: AgentHello, reader: jspb.BinaryReader): AgentHello;
}

export namespace AgentHello {
  export type AsObject = {
    deviceId: string,
    agentVersion: string,
    protocolVersion: string,
  }
}

export class WatchRequest extends jspb.Message {
  getSessionId(): string;
  setSessionId(value: string): void;

  getPairingCode(): string;
  setPairingCode(value: string): void;

  getLastSequence(): number;
  setLastSequence(value: number): void;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): WatchRequest.AsObject;
  static toObject(includeInstance: boolean, msg: WatchRequest): WatchRequest.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: WatchRequest, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): WatchRequest;
  static deserializeBinaryFromReader(message: WatchRequest, reader: jspb.BinaryReader): WatchRequest;
}

export namespace WatchRequest {
  export type AsObject = {
    sessionId: string,
    pairingCode: string,
    lastSequence: number,
  }
}

