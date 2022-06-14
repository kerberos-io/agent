import React from 'react';
import {
  Breadcrumb,
  VideoContainer,
  VideoCard,
  ControlBar,
  Input,
} from '@kerberos-io/ui';
import styles from './Media.scss';

// eslint-disable-next-line react/prefer-stateless-function
class Media extends React.Component {
  render() {
    return (
      <div className={styles.dashboard}>
        <Breadcrumb
          title="Recordings"
          level1="All your recordings in a single place"
          level1Link=""
        >
          {/* <Link to="/deployments">
            <Button
              label="Add Kerberos Agent"
              icon="plus-circle"
              type="default"
            />
    </Link> */}
        </Breadcrumb>

        <ControlBar>
          <Input
            iconleft="search"
            onChange={() => {}}
            placeholder="Search media..."
            layout="controlbar"
            type="text"
          />
        </ControlBar>

        <VideoContainer cols={4} isVideoWall={false}>
          {Array(12)
            .fill(4)
            .map(() => (
              <VideoCard
                key="card"
                headerStatus="hub"
                headerStatusTitle="Live"
                camera="Camera 12-Outside"
                isVideoWall={false}
                isMediaWall
                videoSrc="https://www.w3schools.com/html/mov_bbb.mp4"
                videoStatus="recording"
                videoStatusTitle="live"
                duration="5:45"
                hours="17:35 - 17:36"
                month="Mar 26"
              />
            ))}
        </VideoContainer>
      </div>
    );
  }
}
export default Media;
