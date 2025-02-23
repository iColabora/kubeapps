import { mount } from "enzyme";
import ChartHeader from "./ChartHeader";
import {
  AvailablePackageDetail,
  Context,
  PackageAppVersion,
} from "gen/kubeappsapis/core/packages/v1alpha1/packages";

const testProps: any = {
  chartAttrs: {
    shortDescription: "A Test Chart",
    name: "test",
    categories: [""],
    displayName: "foo",
    iconUrl: "api/assetsvc/test.jpg",
    repoUrl: "",
    homeUrl: "",
    sourceUrls: [],
    longDescription: "",
    availablePackageRef: {
      identifier: "testrepo/foo",
      context: { cluster: "default", namespace: "kubeapps" } as Context,
    },
    valuesSchema: "",
    defaultValues: "",
    maintainers: [],
    readme: "",
    version: {
      pkgVersion: "1.2.3",
      appVersion: "4.5.6",
    },
  } as AvailablePackageDetail,
  versions: [
    {
      pkgVersion: "0.1.2",
      appVersion: "1.2.3",
    },
  ] as PackageAppVersion[],
  onSelect: jest.fn(),
};

it("renders a header for the chart", () => {
  const wrapper = mount(<ChartHeader {...testProps} />);
  expect(wrapper.text()).toContain("testrepo/test");
});

it("displays the appVersion", () => {
  const wrapper = mount(<ChartHeader {...testProps} />);
  expect(wrapper.text()).toContain("1.2.3");
});

it("uses the icon", () => {
  const wrapper = mount(<ChartHeader {...testProps} />);
  const icon = wrapper.find("img").filterWhere(i => i.prop("alt") === "icon");
  expect(icon.exists()).toBe(true);
  expect(icon.props()).toMatchObject({ src: "api/assetsvc/test.jpg" });
});

it("uses the first version as default in the select input", () => {
  const versions: PackageAppVersion[] = [
    {
      pkgVersion: "1.2.3",
      appVersion: "10.0.0",
    },
    {
      pkgVersion: "1.2.4",
      appVersion: "10.0.0",
    },
  ];
  const wrapper = mount(<ChartHeader {...testProps} versions={versions} />);
  expect(wrapper.find("select").prop("value")).toBe("1.2.3");
});

it("uses the current version as default in the select input", () => {
  const versions = [
    {
      attributes: {
        version: "1.2.3",
      },
    },
    {
      attributes: {
        version: "1.2.4",
      },
    },
  ];
  const wrapper = mount(<ChartHeader {...testProps} versions={versions} currentVersion="1.2.4" />);
  expect(wrapper.find("select").prop("value")).toBe("1.2.4");
});
