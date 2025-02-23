import React, { useMemo } from "react";
import { IBasicFormParam, IStoreState } from "shared/types";
import { CustomComponent } from "../../../RemoteComponent";
import { useSelector } from "react-redux";
export interface ICustomParamProps {
  param: IBasicFormParam;
  handleBasicFormParamChange: (
    p: IBasicFormParam,
  ) => (e: React.FormEvent<HTMLInputElement>) => void;
}

export default function CustomFormComponentLoader({
  param,
  handleBasicFormParamChange,
}: ICustomParamProps) {
  // Fetches the custom-component bundle served by the dashboard nginx

  const {
    config: { remoteComponentsUrl },
  } = useSelector((state: IStoreState) => state);

  const url = remoteComponentsUrl
    ? remoteComponentsUrl
    : `${window.location.origin}/custom_components.js`;

  return useMemo(
    () => (
      <CustomComponent
        url={url}
        param={param}
        handleBasicFormParamChange={handleBasicFormParamChange}
      />
    ),
    [handleBasicFormParamChange, param, url],
  );
}
