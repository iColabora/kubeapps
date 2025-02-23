import { CdsButton } from "@cds/react/button";
import actions from "actions";
import ConfirmDialog from "components/ConfirmDialog/ConfirmDialog";
import Alert from "components/js/Alert";
import {
  InstalledPackageStatus,
  InstalledPackageStatus_StatusReason,
} from "gen/kubeappsapis/core/packages/v1alpha1/packages";
import { act } from "react-dom/test-utils";
import * as ReactRedux from "react-redux";
import ReactTooltip from "react-tooltip";
import { defaultStore, getStore, mountWrapper } from "shared/specs/mountWrapper";
import { DeleteError } from "shared/types";
import DeleteButton from "./DeleteButton";

const defaultProps = {
  cluster: "default",
  namespace: "kubeapps",
  releaseName: "foo",
  releaseStatus: null,
};

let spyOnUseDispatch: jest.SpyInstance;
const kubeaActions = { ...actions.kube };
beforeEach(() => {
  actions.apps = {
    ...actions.apps,
    deleteApp: jest.fn(),
  };
  const mockDispatch = jest.fn();
  spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
});

afterEach(() => {
  actions.kube = { ...kubeaActions };
  spyOnUseDispatch.mockRestore();
});

it("deletes an application", async () => {
  const deleteApp = jest.fn();
  actions.apps.deleteApp = deleteApp;
  const wrapper = mountWrapper(defaultStore, <DeleteButton {...defaultProps} />);
  act(() => {
    (wrapper.find(CdsButton).prop("onClick") as any)();
  });
  wrapper.update();
  expect(wrapper.find(ConfirmDialog).prop("modalIsOpen")).toBe(true);
  await act(async () => {
    await (
      wrapper
        .find(".btn")
        .filterWhere(b => b.text() === "Delete")
        .prop("onClick") as any
    )();
  });
  expect(deleteApp).toHaveBeenCalledWith(
    defaultProps.cluster,
    defaultProps.namespace,
    defaultProps.releaseName,
    true,
  );
});

it("renders an error", async () => {
  const store = getStore({ apps: { error: new DeleteError("Boom!") } });
  const wrapper = mountWrapper(store, <DeleteButton {...defaultProps} />);
  // Open modal
  act(() => {
    (wrapper.find(CdsButton).prop("onClick") as any)();
  });
  wrapper.update();

  expect(wrapper.find(Alert)).toIncludeText("Boom!");
});

it("should render a disabled button if when passing an in-progress status", async () => {
  const disabledProps = {
    ...defaultProps,
    releaseStatus: {
      ready: false,
      reason: InstalledPackageStatus_StatusReason.STATUS_REASON_PENDING,
      userReason: "Pending",
    } as InstalledPackageStatus,
  };
  const wrapper = mountWrapper(defaultStore, <DeleteButton {...disabledProps} />);

  expect(wrapper.find(CdsButton)).toBeDisabled();
  expect(wrapper.find(ReactTooltip)).toExist();
});
