import Tabs from "components/Tabs";
import { shallow } from "enzyme";
import ResourceRef from "shared/ResourceRef";
import ResourceTabs from "./ResourceTabs";

const sampleResourceRef = {
  apiVersion: "v1",
  kind: "Deployment",
  name: "foo",
  namespace: "default",
} as ResourceRef;

it("renders resource tables for all the resources", () => {
  const wrapper = shallow(
    <ResourceTabs
      {...{
        deployments: [sampleResourceRef],
        statefulsets: [sampleResourceRef],
        daemonsets: [sampleResourceRef],
        services: [sampleResourceRef],
        secrets: [sampleResourceRef],
        otherResources: [sampleResourceRef],
      }}
    />,
  );
  expect(wrapper.find(Tabs).prop("columns").length).toBe(6);
  expect(wrapper.find(Tabs).prop("data").length).toBe(6);
});

it("renders some resource tables", () => {
  const wrapper = shallow(
    <ResourceTabs
      {...{
        deployments: [sampleResourceRef],
        statefulsets: [],
        daemonsets: [],
        services: [],
        secrets: [sampleResourceRef],
        otherResources: [],
      }}
    />,
  );
  expect(wrapper.find(Tabs).prop("columns").length).toBe(2);
  expect(wrapper.find(Tabs).prop("data").length).toBe(2);
});
