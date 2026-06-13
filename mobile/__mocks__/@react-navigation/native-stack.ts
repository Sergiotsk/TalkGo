import React from 'react';

type ScreenConfig = {
  name: string;
  component: React.ComponentType;
};

export const createNativeStackNavigator = jest.fn(() => {
  const screens: ScreenConfig[] = [];
  let initialRoute: string | undefined;

  const Screen = ({ name, component }: ScreenConfig) => {
    screens.push({ name, component });
    return null;
  };

  const Navigator = ({
    children,
    initialRouteName,
  }: {
    children: React.ReactNode;
    initialRouteName?: string;
  }) => {
    initialRoute = initialRouteName;
    // Collect all Screen children first
    const collected: ScreenConfig[] = [];
    React.Children.forEach(children, (child) => {
      if (React.isValidElement(child)) {
        const props = child.props as ScreenConfig;
        collected.push({ name: props.name, component: props.component });
      }
    });

    // Only render the initial screen
    const target = initialRoute
      ? collected.find((s) => s.name === initialRoute)
      : collected[0];

    if (!target) return React.createElement(React.Fragment);
    return React.createElement(target.component);
  };

  return { Navigator, Screen };
});
