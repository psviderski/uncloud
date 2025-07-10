// Import the original mapper
import MDXComponents from '@theme-original/MDXComponents';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

export default {
    // Re-use the default mapping
    ...MDXComponents,
    // Global import the <Tabs> and <TabItem> components
    Tabs,
    TabItem,
};
