// package: 
// file: app/dashboard/dashboard.proto

import * as jspb from "google-protobuf";

export class DashboardPartitionState extends jspb.Message {
  getCumulativeTimesteps(): number;
  setCumulativeTimesteps(value: number): void;

  getPartitionIndex(): number;
  setPartitionIndex(value: number): void;

  clearStateList(): void;
  getStateList(): Array<number>;
  setStateList(value: Array<number>): void;
  addState(value: number, index?: number): number;

  serializeBinary(): Uint8Array;
  toObject(includeInstance?: boolean): DashboardPartitionState.AsObject;
  static toObject(includeInstance: boolean, msg: DashboardPartitionState): DashboardPartitionState.AsObject;
  static extensions: {[key: number]: jspb.ExtensionFieldInfo<jspb.Message>};
  static extensionsBinary: {[key: number]: jspb.ExtensionFieldBinaryInfo<jspb.Message>};
  static serializeBinaryToWriter(message: DashboardPartitionState, writer: jspb.BinaryWriter): void;
  static deserializeBinary(bytes: Uint8Array): DashboardPartitionState;
  static deserializeBinaryFromReader(message: DashboardPartitionState, reader: jspb.BinaryReader): DashboardPartitionState;
}

export namespace DashboardPartitionState {
  export type AsObject = {
    cumulativeTimesteps: number,
    partitionIndex: number,
    stateList: Array<number>,
  }
}

