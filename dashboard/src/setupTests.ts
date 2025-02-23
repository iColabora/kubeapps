import Enzyme from "enzyme";
import Adapter from "@wojtekmaj/enzyme-adapter-react-17";

import "jest-enzyme";
import { WebSocket } from "mock-socket";
import "raf/polyfill"; // polyfill for requestAnimationFrame

Enzyme.configure({ adapter: new Adapter() });

// Mock browser specific APIs like localstorage or Websocket
jest.spyOn(window.localStorage.__proto__, "clear");
jest.spyOn(window.localStorage.__proto__, "getItem");
jest.spyOn(window.localStorage.__proto__, "setItem");
jest.spyOn(window.localStorage.__proto__, "removeItem");

(global as any).WebSocket = WebSocket;
