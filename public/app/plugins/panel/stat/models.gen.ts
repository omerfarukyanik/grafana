// Code generated - EDITING IS FUTILE. DO NOT EDIT.
//
// Generated by:
//     public/app/plugins/gen.go
// Using jennies:
//     TSTypesJenny
//     PluginTSTypesJenny
//
// Run 'make gen-cue' from repository root to regenerate.

import * as ui from '@grafana/schema';

export const PanelModelVersion = Object.freeze([0, 0]);

export interface PanelOptions extends ui.SingleStatBaseOptions {
  colorMode: ui.BigValueColorMode;
  graphMode: ui.BigValueGraphMode;
  justifyMode: ui.BigValueJustifyMode;
  textMode: ui.BigValueTextMode;
}

export const defaultPanelOptions: Partial<PanelOptions> = {
  colorMode: ui.BigValueColorMode.Value,
  graphMode: ui.BigValueGraphMode.Area,
  justifyMode: ui.BigValueJustifyMode.Auto,
  textMode: ui.BigValueTextMode.Auto,
};
