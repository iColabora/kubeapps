import actions from "actions";
import FilterGroup from "components/FilterGroup/FilterGroup";
import InfoCard from "components/InfoCard/InfoCard";
import Alert from "components/js/Alert";
import LoadingWrapper from "components/LoadingWrapper";
import { AvailablePackageSummary, Context } from "gen/kubeappsapis/core/packages/v1alpha1/packages";
import { createMemoryHistory } from "history";
import React from "react";
import { act } from "react-dom/test-utils";
import * as ReactRedux from "react-redux";
import * as ReactRouter from "react-router";
import { MemoryRouter, Route, Router } from "react-router";
import { IConfigState } from "reducers/config";
import { IOperatorsState } from "reducers/operators";
import { IAppRepositoryState } from "reducers/repos";
import { getStore, initialState, mountWrapper } from "shared/specs/mountWrapper";
import { IAppRepository, IChartState, IClusterServiceVersion } from "../../shared/types";
import SearchFilter from "../SearchFilter/SearchFilter";
import Catalog, { filterNames } from "./Catalog";
import CatalogItems from "./CatalogItems";
import ChartCatalogItem from "./ChartCatalogItem";

const defaultChartState = {
  isFetching: false,
  hasFinishedFetching: false,
  selected: {} as IChartState["selected"],
  deployed: {} as IChartState["deployed"],
  items: [],
  categories: [],
  size: 20,
} as IChartState;
const defaultProps = {
  charts: defaultChartState,
  repo: "",
  filter: {},
  cluster: initialState.config.kubeappsCluster,
  namespace: "kubeapps",
  kubeappsNamespace: "kubeapps",
  csvs: [],
};
const availablePkgSummary1: AvailablePackageSummary = {
  name: "foo",
  categories: [""],
  displayName: "foo",
  iconUrl: "",
  latestVersion: { appVersion: "v1.0.0", pkgVersion: "" },
  shortDescription: "",
  availablePackageRef: {
    identifier: "foo/foo",
    context: { cluster: "", namespace: "chart-namespace" } as Context,
  },
};
const availablePkgSummary2: AvailablePackageSummary = {
  name: "bar",
  categories: ["Database"],
  displayName: "bar",
  iconUrl: "",
  latestVersion: { appVersion: "v2.0.0", pkgVersion: "" },
  shortDescription: "",
  availablePackageRef: {
    identifier: "bar/bar",
    context: { cluster: "", namespace: "chart-namespace" } as Context,
  },
};
const csv = {
  metadata: {
    name: "test-csv",
  },
  spec: {
    provider: {
      name: "me",
    },
    icon: [{ base64data: "data", mediatype: "img/png" }],
    customresourcedefinitions: {
      owned: [
        {
          name: "foo-cluster",
          displayName: "foo-cluster",
          version: "v1.0.0",
          description: "a meaningful description",
        },
      ],
    },
  },
} as IClusterServiceVersion;

const defaultState = {
  charts: defaultChartState,
  operators: { csvs: [] } as Partial<IOperatorsState>,
  repos: { repos: [] } as Partial<IAppRepositoryState>,
  config: {
    kubeappsCluster: defaultProps.cluster,
    kubeappsNamespace: defaultProps.kubeappsNamespace,
  } as IConfigState,
};

const populatedChartState = {
  ...defaultChartState,
  items: [availablePkgSummary1, availablePkgSummary2],
};
const populatedState = {
  ...defaultState,
  charts: populatedChartState,
  operators: { csvs: [csv] },
};

let spyOnUseDispatch: jest.SpyInstance;
let spyOnUseHistory: jest.SpyInstance;

beforeEach(() => {
  const mockDispatch = jest.fn();
  spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
  spyOnUseHistory = jest
    .spyOn(ReactRouter, "useHistory")
    .mockReturnValue({ push: jest.fn() } as any);
});

afterEach(() => {
  jest.restoreAllMocks();
  spyOnUseDispatch.mockRestore();
  spyOnUseHistory.mockRestore();
});

const routePathParam = `/c/${defaultProps.cluster}/ns/${defaultProps.namespace}/catalog`;
const routePath = "/c/:cluster/ns/:namespace/catalog";
const history = createMemoryHistory({ initialEntries: [routePathParam] });

it("retrieves csvs in the namespace", () => {
  const getCSVs = jest.fn();
  actions.operators.getCSVs = getCSVs;

  mountWrapper(
    getStore(populatedState),
    <Router history={history}>
      <Route path={routePath}>
        <Catalog />
      </Route>
    </Router>,
  );

  expect(getCSVs).toHaveBeenCalledWith(defaultProps.cluster, defaultProps.namespace);
});

it("shows all the elements", () => {
  const wrapper = mountWrapper(getStore(populatedState), <Catalog />);
  expect(wrapper.find(InfoCard)).toHaveLength(3);
});

it("should not render a message if there are no elements in the catalog but the fetching hasn't ended", () => {
  const wrapper = mountWrapper(getStore(defaultState), <Catalog />);
  const message = wrapper.find(".empty-catalog");
  expect(message).not.toExist();
  expect(message).not.toIncludeText("The current catalog is empty");
});

it("should render a message if there are no elements in the catalog and the fetching has ended", () => {
  const wrapper = mountWrapper(
    getStore({ ...defaultState, charts: { hasFinishedFetching: true } }),
    <Catalog />,
  );
  wrapper.setProps({ searchFilter: "" });
  const message = wrapper.find(".empty-catalog");
  expect(message).toExist();
  expect(message).toIncludeText("The current catalog is empty");
});

it("should render a spinner if there are no elements but it's still fetching", () => {
  const wrapper = mountWrapper(
    getStore({ ...defaultState, charts: { hasFinishedFetching: false } }),
    <Catalog />,
  );
  expect(wrapper.find(LoadingWrapper)).toExist();
});

it("should not render a spinner if there are no elements and it finished fetching", () => {
  const wrapper = mountWrapper(
    getStore({ ...defaultState, charts: { hasFinishedFetching: true } }),
    <Catalog />,
  );
  expect(wrapper.find(LoadingWrapper)).not.toExist();
});

it("should render a spinner if there already pending elements", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: false } }),
    <Catalog />,
  );
  expect(wrapper.find(LoadingWrapper)).toExist();
});

it("should not render a message if only operators are selected", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: true } }),
    <MemoryRouter initialEntries={[routePathParam + "?Operators=bar"]}>
      <Route path={routePath}>
        <Catalog />
      </Route>
    </MemoryRouter>,
  );
  expect(wrapper.find(LoadingWrapper)).not.toExist();
});

it("should not render a message if there are no more elements", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: true } }),
    <Catalog />,
  );
  const message = wrapper.find(".endPageMessage");
  expect(message).not.toExist();
});

it("should not render a message if there are no more elements but it's searching", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: true } }),
    <MemoryRouter initialEntries={[routePathParam + "?Search=bar"]}>
      <Route path={routePath}>
        <Catalog />
      </Route>
    </MemoryRouter>,
  );
  const message = wrapper.find(".endPageMessage");
  expect(message).not.toExist();
});

it("should render the scroll handler if not finished", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: false } }),
    <Catalog />,
  );
  const scroll = wrapper.find(".scrollHandler");
  expect(scroll).toExist();
  expect(scroll).toHaveProperty("ref");
});

it("should not render the scroll handler if finished", () => {
  const wrapper = mountWrapper(
    getStore({ ...populatedState, charts: { hasFinishedFetching: true } }),
    <Catalog />,
  );
  const scroll = wrapper.find(".scrollHandler");
  expect(scroll).not.toExist();
});

it("should render an error if it exists", () => {
  const charts = {
    ...defaultChartState,
    selected: {
      error: new Error("Boom!"),
    },
  } as any;
  const wrapper = mountWrapper(getStore({ ...populatedState, charts: charts }), <Catalog />);
  const error = wrapper.find(Alert);
  expect(error.prop("theme")).toBe("danger");
  expect(error).toIncludeText("Boom!");
});

it("behaves like a loading wrapper", () => {
  const charts = { isFetching: true, items: [], categories: [], selected: {} } as any;
  const wrapper = mountWrapper(getStore({ ...populatedState, charts: charts }), <Catalog />);
  expect(wrapper.find(LoadingWrapper)).toExist();
});

it("transforms the received '__' in query params into a ','", () => {
  const wrapper = mountWrapper(
    getStore(populatedState),
    <MemoryRouter initialEntries={[routePathParam + "?Provider=Lightbend__%20Inc."]}>
      <Route path={routePath}>
        <Catalog />
      </Route>
    </MemoryRouter>,
  );
  expect(wrapper.find(".label-info").text()).toBe("Provider: Lightbend, Inc. ");
});

describe("filters by the searched item", () => {
  let spyOnUseDispatch: jest.SpyInstance;
  let spyOnUseEffect: jest.SpyInstance;

  afterEach(() => {
    spyOnUseDispatch.mockRestore();
    spyOnUseEffect.mockRestore();
  });

  it("filters modifying the search box", () => {
    const fetchCharts = jest.fn();
    actions.charts.fetchCharts = fetchCharts;
    const mockDispatch = jest.fn();
    const mockUseEffect = jest.fn();

    spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
    spyOnUseEffect = jest.spyOn(React, "useEffect").mockReturnValue(mockUseEffect as any);

    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam + "?Search=bar"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    act(() => {
      (wrapper.find(SearchFilter).prop("onChange") as any)("bar");
    });
    wrapper.update();
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Search=bar"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });
});

describe("filters by application type", () => {
  let spyOnUseDispatch: jest.SpyInstance;
  const mockDispatch = jest.fn();

  beforeEach(() => {
    spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
  });

  afterEach(() => {
    spyOnUseDispatch.mockRestore();
    mockDispatch.mockRestore();
  });

  it("doesn't show the filter if there are no csvs", () => {
    const wrapper = mountWrapper(getStore(defaultState), <Catalog />);
    expect(
      wrapper.find(FilterGroup).findWhere(g => g.prop("name") === filterNames.TYPE),
    ).not.toExist();
  });

  it("filters only charts", () => {
    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam + "?Type=Charts"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(2);
  });

  it("push filter for only charts", () => {
    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "Charts");
    expect(input).toHaveLength(1);
    input.simulate("change", { target: { value: "Charts", checked: true } });

    // It should have pushed with the filter
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Type=Charts"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });

  it("filters only operators", () => {
    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam + "?Type=Operators"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });

  it("push filter for only operators", () => {
    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "Operators");
    expect(input).toHaveLength(1);
    input.simulate("change", { target: { value: "Operators", checked: true } });

    // It should have pushed with the filter
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Type=Operators"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });
});

describe("pagination and chart fetching", () => {
  it("sets the initial state page to 0 before fetching charts", () => {
    const fetchCharts = jest.fn();
    actions.charts.fetchCharts = fetchCharts;
    // const resetRequestCharts = jest.fn();

    const charts = {
      ...defaultChartState,
      hasFinishedFetching: false,
      isFetching: false,
      items: [],
    } as any;
    const wrapper = mountWrapper(
      getStore({ ...populatedState, charts: charts }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );

    expect(wrapper.find(CatalogItems).prop("page")).toBe(0);
    expect(wrapper.find(ChartCatalogItem).length).toBe(0);
    expect(fetchCharts).toHaveBeenNthCalledWith(1, "default-cluster", "kubeapps", "", 0, 20, "");
    // TODO(agamez): check whether it should be called
    // expect(resetRequestCharts).toHaveBeenNthCalledWith(1);
  });

  it("sets the state page when fetching charts", () => {
    const fetchCharts = jest.fn();
    actions.charts.fetchCharts = fetchCharts;
    // const resetRequestCharts = jest.fn();

    const charts = {
      ...defaultChartState,
      hasFinishedFetching: false,
      isFetching: true,
      items: [availablePkgSummary1],
    } as any;
    const wrapper = mountWrapper(
      getStore({ ...populatedState, charts: charts }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );

    expect(wrapper.find(CatalogItems).prop("page")).toBe(0);
    expect(wrapper.find(ChartCatalogItem).length).toBe(1);
    expect(fetchCharts).toHaveBeenCalledWith("default-cluster", "kubeapps", "", 0, 20, "");
    // TODO(agamez): check whether it should be called
    // expect(resetRequestCharts).toHaveBeenCalledWith();
  });

  it("items are translated to CatalogItems after fetching charts", () => {
    const fetchCharts = jest.fn();
    actions.charts.fetchCharts = fetchCharts;
    // const resetRequestCharts = jest.fn();

    const charts = {
      ...defaultChartState,
      hasFinishedFetching: true,
      isFetching: false,
      items: [availablePkgSummary1, availablePkgSummary2],
    } as any;
    const wrapper = mountWrapper(
      getStore({ ...populatedState, charts: charts }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );

    expect(wrapper.find(CatalogItems).prop("page")).toBe(0);
    expect(wrapper.find(ChartCatalogItem).length).toBe(2);
    expect(fetchCharts).toHaveBeenCalledWith("default-cluster", "kubeapps", "", 0, 20, "");
    // TODO(agamez): check whether it should be called
    // expect(resetRequestCharts).toHaveBeenCalledWith();
  });

  describe("pagination", () => {
    let spyOnUseState: jest.SpyInstance;
    const setState = jest.fn();
    const setPage = jest.fn();

    beforeEach(() => {
      spyOnUseState = jest
        .spyOn(React, "useState")
        /* @ts-expect-error: Argument of type '(init: any) => any' is not assignable to parameter of type '() => [unknown, Dispatch<unknown>]' */
        .mockImplementation((init: any) => {
          if (init === false) {
            // Mocking the result of hasLoadedFirstPage to simulate that is already loaded
            return [true, setState];
          }
          if (init === 0) {
            // Mocking the result of setPage to ensure it's called
            return [0, setPage];
          }
          return [init, setState];
        });

      // Mock intersection observer
      const observe = jest.fn();
      const unobserve = jest.fn();

      window.IntersectionObserver = jest.fn(callback => {
        (callback as (e: any) => void)([{ isIntersecting: true }]);
        return { observe, unobserve } as any;
      });
    });

    afterEach(() => {
      spyOnUseState.mockRestore();
    });

    it("changes page", () => {
      const charts = {
        ...defaultChartState,
        hasFinishedFetching: false,
        isFetching: false,
        items: [],
      } as any;

      mountWrapper(
        getStore({ ...populatedState, charts: charts }),
        <MemoryRouter initialEntries={[routePathParam]}>
          <Route path={routePath}>
            <Catalog />
          </Route>
        </MemoryRouter>,
      );
      expect(setPage).toHaveBeenCalledWith(0);
    });
    // TODO(agamez): add a test case covering it "resets page when one of the filters changes"
    // https://github.com/kubeapps/kubeapps/pull/2264/files/0d3c77448543668255809bf05039aca704cf729f..22343137efb1c2292b0aa4795f02124306cb055e#r565486271
  });
});

describe("filters by application repository", () => {
  const mockDispatch = jest.fn();
  let spyOnUseDispatch: jest.SpyInstance;
  let fetchRepos: jest.SpyInstance;

  beforeEach(() => {
    spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
    // Can't just assign a mock fn to actions.repos.fetchRepos because it is (correctly) exported
    // as a const fn.
    fetchRepos = jest.spyOn(actions.repos, "fetchRepos").mockImplementation(() => {
      return jest.fn();
    });
  });

  afterEach(() => {
    mockDispatch.mockRestore();
    spyOnUseDispatch.mockRestore();
    fetchRepos.mockRestore();
  });

  it("doesn't show the filter if there are no apps", () => {
    const wrapper = mountWrapper(
      getStore(defaultState),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(
      wrapper.find(FilterGroup).findWhere(g => g.prop("name") === filterNames.REPO),
    ).not.toExist();
  });

  it("filters by repo", () => {
    const wrapper = mountWrapper(
      getStore(populatedState),
      <MemoryRouter initialEntries={[routePathParam + "?Repository=foo"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });

  it("push filter for repo", () => {
    const wrapper = mountWrapper(
      getStore({
        ...populatedState,
        repos: { repos: [{ metadata: { name: "foo" } } as IAppRepository] },
      }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );

    // The repo name is "foo"
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "foo");
    input.simulate("change", { target: { value: "foo" } });
    // It should have pushed with the filter
    expect(fetchRepos).toHaveBeenCalledWith("kubeapps");
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Repository=foo"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });

  it("push filter for repo in other ns", () => {
    const wrapper = mountWrapper(
      getStore({
        ...populatedState,
        repos: { repos: [{ metadata: { name: "foo" } } as IAppRepository] },
      }),
      <MemoryRouter initialEntries={[`/c/${defaultProps.cluster}/ns/my-ns/catalog`]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );

    // The repo name is "foo", the ns name is "my-ns"
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "foo");
    input.simulate("change", { target: { value: "foo" } });

    // It should have pushed with the filter
    expect(fetchRepos).toHaveBeenCalledWith("my-ns", true);
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/my-ns/catalog?Repository=foo"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });
});

describe("filters by operator provider", () => {
  const mockDispatch = jest.fn();

  beforeEach(() => {
    spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
  });
  afterEach(() => {
    spyOnUseDispatch.mockRestore();
    mockDispatch.mockRestore();
  });

  it("doesn't show the filter if there are no csvs", () => {
    const wrapper = mountWrapper(getStore(defaultState), <Catalog />);
    expect(
      wrapper.find(FilterGroup).findWhere(g => g.prop("name") === filterNames.OPERATOR_PROVIDER),
    ).not.toExist();
  });

  const csv2 = {
    metadata: {
      name: "csv2",
    },
    spec: {
      ...csv.spec,
      provider: {
        name: "you",
      },
    },
  } as any;

  it("push filter for operator provider", () => {
    const wrapper = mountWrapper(
      getStore({
        ...populatedState,
        operators: { csvs: [csv, csv2] },
      }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "you");
    input.simulate("change", { target: { value: "you" } });
    // It should have pushed with the filter
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Provider=you"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });

  it("push filter for operator provider with comma", () => {
    const wrapper = mountWrapper(
      getStore({
        ...populatedState,
        operators: { csvs: [csv, csv2] },
      }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "you");
    input.simulate("change", { target: { value: "you, inc" } });
    // It should have pushed with the filter
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Provider=you__%20inc"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });

  it("filters by operator provider", () => {
    const wrapper = mountWrapper(
      getStore({
        ...populatedState,
        operators: { csvs: [csv, csv2] },
      }),
      <MemoryRouter initialEntries={[routePathParam + "?Provider=you"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });
});

describe("filters by category", () => {
  const mockDispatch = jest.fn();

  beforeEach(() => {
    spyOnUseDispatch = jest.spyOn(ReactRedux, "useDispatch").mockReturnValue(mockDispatch);
  });
  afterEach(() => {
    spyOnUseDispatch.mockRestore();
    mockDispatch.mockRestore();
  });
  it("renders a Unknown category if not set", () => {
    const charts = {
      ...defaultChartState,
      items: [availablePkgSummary1],
      categories: [availablePkgSummary1.categories[0]],
    };
    const wrapper = mountWrapper(
      getStore({ ...populatedState, charts: charts }),
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find("input").findWhere(i => i.prop("value") === "Unknown")).toExist();
  });

  it("push filter for category", () => {
    const charts = {
      ...defaultChartState,
      items: [availablePkgSummary1, availablePkgSummary2],
      categories: [availablePkgSummary1.categories[0], availablePkgSummary2.categories[0]],
    };
    const store = getStore({ ...defaultState, charts: charts });
    const wrapper = mountWrapper(
      store,
      <MemoryRouter initialEntries={[routePathParam]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(2);
    const input = wrapper.find("input").findWhere(i => i.prop("value") === "Database");
    input.simulate("change", { target: { value: "Database" } });
    // It should have pushed with the filter
    expect(mockDispatch).toHaveBeenCalledWith({
      payload: {
        args: ["/c/default-cluster/ns/kubeapps/catalog?Category=Database"],
        method: "push",
      },
      type: "@@router/CALL_HISTORY_METHOD",
    });
  });

  it("filters a category", () => {
    const charts = {
      ...defaultChartState,
      items: [availablePkgSummary1, availablePkgSummary2],
      categories: [availablePkgSummary1.categories[0], availablePkgSummary2.categories[0]],
    };
    const wrapper = mountWrapper(
      getStore({ ...populatedState, charts: charts }),
      <MemoryRouter initialEntries={[routePathParam + "?Category=Database"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });

  it("filters an operator category", () => {
    const csvWithCat = {
      ...csv,
      metadata: {
        name: "csv-cat",
        annotations: {
          categories: "E-Learning",
        },
      },
    } as any;
    const wrapper = mountWrapper(
      getStore({ ...populatedState, operators: { csvs: [csv, csvWithCat] } }),
      <MemoryRouter initialEntries={[routePathParam + "?Category=E-Learning"]}>
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });

  it("filters operator categories", () => {
    const csvWithCat = {
      ...csv,
      metadata: {
        name: "csv-cat",
        annotations: {
          categories: "DeveloperTools, Infrastructure",
        },
      },
    } as any;
    const wrapper = mountWrapper(
      getStore({ ...populatedState, operators: { csvs: [csv, csvWithCat] } }),
      <MemoryRouter
        initialEntries={[routePathParam + "?Category=Developer%20Tools,Infrastructure"]}
      >
        <Route path={routePath}>
          <Catalog />
        </Route>
      </MemoryRouter>,
    );
    expect(wrapper.find(InfoCard)).toHaveLength(1);
  });
});
